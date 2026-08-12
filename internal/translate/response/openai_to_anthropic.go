// Package response implements response/stream translators. It ports 9router's
// translator/response/openai-to-claude.js into Go.
package response

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/translate"
	"github.com/oniharnantyo/tinyroute/internal/translate/concerns"
)

func init() {
	translate.Register("anthropic", "openai", nil, openAIToAnthropic{})
}

// openAIToAnthropic converts OpenAI chat-completions response chunks (streaming
// SSE or a single non-streaming body) into Anthropic message events.
type openAIToAnthropic struct{}

// openAIChunk mirrors the fields of an OpenAI chunk we consume.
type openAIChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		Message *struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens        int64 `json:"prompt_tokens"`
		CompletionTokens    int64 `json:"completion_tokens"`
		PromptTokensDetails *struct {
			CachedTokens        int64 `json:"cached_tokens"`
			CacheCreationTokens int64 `json:"cache_creation_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

func (t openAIToAnthropic) TranslateResponse(chunk []byte, state *core.StreamState) ([][]byte, error) {
	if chunk == nil {
		return t.drain(state), nil
	}

	var c openAIChunk
	if err := json.Unmarshal(chunk, &c); err != nil {
		// Non-JSON line (e.g. [DONE] or an injected error): skip silently.
		return nil, nil
	}

	var frames [][]byte

	if !state.MessageStartSent {
		if state.MessageID == "" || !strings.HasPrefix(state.MessageID, "msg_") {
			state.MessageID = synthID("msg")
		}
		if c.Model != "" {
			state.Model = c.Model
		}
		frames = append(frames, messageStartFrame(state))
		state.MessageStartSent = true
	}

	for _, ch := range c.Choices {
		if ch.Delta.Content != "" {
			state.TextBuffer.WriteString(ch.Delta.Content)
			idx := t.ensureText(state, &frames)
			frames = append(frames, contentBlockDeltaFrame(idx, "text_delta", ch.Delta.Content))
		}
		if ch.Delta.ReasoningContent != "" {
			idx := t.ensureThinking(state, &frames)
			frames = append(frames, contentBlockDeltaFrame(idx, "thinking_delta", ch.Delta.ReasoningContent))
		}
		for _, tc := range ch.Delta.ToolCalls {
			t.ensureTool(state, tc, &frames)
			state.ToolArgBuffers[tc.Index] = append(state.ToolArgBuffers[tc.Index], []byte(tc.Function.Arguments)...)
		}
		// A non-streaming body carries its content in choices[].message.
		if ch.Message != nil && ch.Message.Content != "" {
			state.TextBuffer.WriteString(ch.Message.Content)
			idx := t.ensureText(state, &frames)
			frames = append(frames, contentBlockDeltaFrame(idx, "text_delta", ch.Message.Content))
		}
		if ch.FinishReason != nil {
			state.FinishReason = *ch.FinishReason
			t.closeOrdinaryBlocks(state, &frames)
		}
	}

	if c.Usage != nil {
		var cached, creation int64
		if d := c.Usage.PromptTokensDetails; d != nil {
			cached = d.CachedTokens
			creation = d.CacheCreationTokens
		}
		state.Usage = concerns.UsageFromOpenAI(c.Usage.PromptTokens, c.Usage.CompletionTokens, cached, creation)
	}

	return frames, nil
}

// drain handles the end-of-stream sentinel: close open blocks, emit tool
// arguments, then message_delta + message_stop with the captured usage.
func (t openAIToAnthropic) drain(state *core.StreamState) [][]byte {
	var frames [][]byte
	if !state.MessageStartSent {
		if state.MessageID == "" || !strings.HasPrefix(state.MessageID, "msg_") {
			state.MessageID = synthID("msg")
		}
		frames = append(frames, messageStartFrame(state))
		state.MessageStartSent = true
	}

	t.closeOrdinaryBlocks(state, &frames)
	for i, tc := range state.ToolCalls {
		if tc.Closed {
			continue
		}
		frames = append(frames, inputJSONDeltaFrame(tc.BlockIndex, sanitizeToolArgs(state.ToolArgBuffers[i])))
		frames = append(frames, contentBlockStopFrame(tc.BlockIndex))
		tc.Closed = true
		state.ToolCalls[i] = tc
	}

	if state.MessageStartSent && !state.MessageClosed {
		usage := state.Usage
		if usage == nil {
			usage = &core.AnthropicUsage{}
		}
		stop := concerns.FromOpenAIFinish(state.FinishReason, "anthropic")
		frames = append(frames, messageDeltaFrame(stop, usage))
		frames = append(frames, mustJSON(map[string]interface{}{"type": "message_stop"}))
		state.MessageClosed = true
	}
	return frames
}

// ensureText opens the text block if needed and returns its index.
func (t openAIToAnthropic) ensureText(state *core.StreamState, frames *[][]byte) int {
	if state.TextBlockOpen {
		return state.TextBlockIndex
	}
	if state.ThinkingBlockOpen {
		*frames = append(*frames, contentBlockStopFrame(state.ThinkingBlockIndex))
		state.ThinkingBlockOpen = false
	}
	state.TextBlockIndex = state.NextBlockIndex
	state.NextBlockIndex++
	state.TextBlockOpen = true
	*frames = append(*frames, contentBlockStartFrame(state.TextBlockIndex, map[string]interface{}{
		"type": "text", "text": "",
	}))
	return state.TextBlockIndex
}

func (t openAIToAnthropic) ensureThinking(state *core.StreamState, frames *[][]byte) int {
	if state.ThinkingBlockOpen {
		return state.ThinkingBlockIndex
	}
	if state.TextBlockOpen {
		*frames = append(*frames, contentBlockStopFrame(state.TextBlockIndex))
		state.TextBlockOpen = false
	}
	state.ThinkingBlockIndex = state.NextBlockIndex
	state.NextBlockIndex++
	state.ThinkingBlockOpen = true
	*frames = append(*frames, contentBlockStartFrame(state.ThinkingBlockIndex, map[string]interface{}{
		"type": "thinking", "thinking": "",
	}))
	return state.ThinkingBlockIndex
}

func (t openAIToAnthropic) ensureTool(state *core.StreamState, tc struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}, frames *[][]byte) int {
	if tcs, ok := state.ToolCalls[tc.Index]; ok {
		return tcs.BlockIndex
	}
	t.closeOrdinaryBlocks(state, frames)
	id := tc.ID
	if id == "" {
		id = synthID("toolu")
	}
	blockIndex := state.NextBlockIndex
	state.NextBlockIndex++
	state.ToolCalls[tc.Index] = core.ToolCallState{
		BlockIndex: blockIndex,
		ID:         id,
		Name:       tc.Function.Name,
	}
	*frames = append(*frames, contentBlockStartFrame(blockIndex, map[string]interface{}{
		"type":  "tool_use",
		"id":    id,
		"name":  tc.Function.Name,
		"input": map[string]interface{}{},
	}))
	return blockIndex
}

// closeOrdinaryBlocks closes any open text/thinking blocks.
func (t openAIToAnthropic) closeOrdinaryBlocks(state *core.StreamState, frames *[][]byte) {
	if state.TextBlockOpen {
		*frames = append(*frames, contentBlockStopFrame(state.TextBlockIndex))
		state.TextBlockOpen = false
	}
	if state.ThinkingBlockOpen {
		*frames = append(*frames, contentBlockStopFrame(state.ThinkingBlockIndex))
		state.ThinkingBlockOpen = false
	}
}

func messageStartFrame(state *core.StreamState) []byte {
	return mustJSON(map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            state.MessageID,
			"type":          "message",
			"role":          "assistant",
			"model":         state.Model,
			"content":       []interface{}{},
			"usage":         map[string]interface{}{"input_tokens": 0, "output_tokens": 0},
			"stop_reason":   nil,
			"stop_sequence": nil,
		},
	})
}

func contentBlockStartFrame(index int, block map[string]interface{}) []byte {
	return mustJSON(map[string]interface{}{
		"type":          "content_block_start",
		"index":         index,
		"content_block": block,
	})
}

func contentBlockDeltaFrame(index int, deltaType, text string) []byte {
	return mustJSON(map[string]interface{}{
		"type":  "content_block_delta",
		"index": index,
		"delta": map[string]interface{}{"type": deltaType, "text": text},
	})
}

func inputJSONDeltaFrame(index int, partial string) []byte {
	return mustJSON(map[string]interface{}{
		"type":  "content_block_delta",
		"index": index,
		"delta": map[string]interface{}{"type": "input_json_delta", "partial_json": partial},
	})
}

func contentBlockStopFrame(index int) []byte {
	return mustJSON(map[string]interface{}{"type": "content_block_stop", "index": index})
}

func messageDeltaFrame(stopReason string, usage *core.AnthropicUsage) []byte {
	return mustJSON(map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]interface{}{
			"input_tokens":                usage.InputTokens,
			"output_tokens":               usage.OutputTokens,
			"cache_read_input_tokens":     usage.CacheReadInputTokens,
			"cache_creation_input_tokens": usage.CacheCreationInputTokens,
		},
	})
}

func sanitizeToolArgs(buf []byte) string {
	s := bytes.TrimSpace(buf)
	if len(s) == 0 {
		return "{}"
	}
	return string(s)
}

// synthID produces "<kind>_<hex>".
func synthID(kind string) string { return kind + "_" + synthRaw(8) }

func synthRaw(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprintf("%08d", n)))
	}
	return hex.EncodeToString(b)
}

func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

// NonStreamingMessageJSON builds a complete Anthropic message JSON response
// object from the accumulated StreamState for non-streaming requests.
func NonStreamingMessageJSON(state *core.StreamState) []byte {
	usage := state.Usage
	if usage == nil {
		usage = &core.AnthropicUsage{}
	}
	stop := concerns.FromOpenAIFinish(state.FinishReason, "anthropic")

	content := []map[string]interface{}{}
	if state.TextBuffer.Len() > 0 {
		content = append(content, map[string]interface{}{
			"type": "text",
			"text": state.TextBuffer.String(),
		})
	}
	for i, tc := range state.ToolCalls {
		var input map[string]interface{}
		_ = json.Unmarshal(bytes.TrimSpace(state.ToolArgBuffers[i]), &input)
		if input == nil {
			input = map[string]interface{}{}
		}
		content = append(content, map[string]interface{}{
			"type":  "tool_use",
			"id":    tc.ID,
			"name":  tc.Name,
			"input": input,
		})
	}

	res := map[string]interface{}{
		"id":            state.MessageID,
		"type":          "message",
		"role":          "assistant",
		"model":         state.Model,
		"content":       content,
		"stop_reason":   stop,
		"stop_sequence": nil,
		"usage":         usage,
	}
	b, _ := json.Marshal(res)
	return b
}
