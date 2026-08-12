// Package response implements response/stream translators.
package response

import (
	"encoding/json"
	"strings"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/translate"
)

func init() {
	translate.Register("openai", "gemini", nil, geminiToOpenAI{})
}

type geminiToOpenAI struct{}

type geminiRespChunk struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text         string `json:"text,omitempty"`
				Thought      bool   `json:"thought,omitempty"`
				FunctionCall *struct {
					Name string                 `json:"name"`
					Args map[string]interface{} `json:"args"`
				} `json:"functionCall,omitempty"`
			} `json:"parts"`
			Role string `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
		Index        int    `json:"index"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int64 `json:"promptTokenCount"`
		CandidatesTokenCount int64 `json:"candidatesTokenCount"`
		TotalTokenCount      int64 `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func (t geminiToOpenAI) TranslateResponse(chunk []byte, state *core.StreamState) ([][]byte, error) {
	if chunk == nil {
		return t.drain(state), nil
	}

	var c geminiRespChunk
	if err := json.Unmarshal(chunk, &c); err != nil {
		return nil, nil
	}

	if state.MessageID == "" {
		state.MessageID = "chatcmpl-" + synthRaw(8)
	}

	var frames [][]byte
	hasFunctionCall := false

	for _, cand := range c.Candidates {
		idx := cand.Index

		var toolCalls []map[string]interface{}
		var textContent strings.Builder
		var reasoningContent strings.Builder

		for _, part := range cand.Content.Parts {
			if part.Thought {
				reasoningContent.WriteString(part.Text)
			} else if part.Text != "" {
				textContent.WriteString(part.Text)
			}
			if part.FunctionCall != nil {
				hasFunctionCall = true
				callID := "call_" + synthRaw(8)
				argsBytes, _ := json.Marshal(part.FunctionCall.Args)
				if len(argsBytes) == 0 {
					argsBytes = []byte("{}")
				}
				funcName := part.FunctionCall.Name
				if state != nil && state.GeminiNameMap != nil {
					funcName = state.GeminiNameMap.Restore(funcName)
				}
				toolCalls = append(toolCalls, map[string]interface{}{
					"index": len(toolCalls),
					"id":    callID,
					"type":  "function",
					"function": map[string]interface{}{
						"name":      funcName,
						"arguments": string(argsBytes),
					},
				})
			}
		}

		delta := map[string]interface{}{"role": "assistant"}
		if reasoningContent.Len() > 0 {
			delta["reasoning_content"] = reasoningContent.String()
		}
		if textContent.Len() > 0 {
			delta["content"] = textContent.String()
		}
		if len(toolCalls) > 0 {
			delta["tool_calls"] = toolCalls
		}

		choice := map[string]interface{}{
			"index": idx,
			"delta": delta,
		}

		if cand.FinishReason != "" {
			fr := mapGeminiFinishReason(cand.FinishReason, hasFunctionCall)
			choice["finish_reason"] = fr
			state.FinishReason = fr
		}

		openAIChunk := map[string]interface{}{
			"id":      state.MessageID,
			"object":  "chat.completion.chunk",
			"created": 0,
			"model":   state.Model,
			"choices": []interface{}{choice},
		}

		if c.UsageMetadata != nil {
			openAIChunk["usage"] = map[string]interface{}{
				"prompt_tokens":     c.UsageMetadata.PromptTokenCount,
				"completion_tokens": c.UsageMetadata.CandidatesTokenCount,
				"total_tokens":      c.UsageMetadata.TotalTokenCount,
			}
			state.Usage = &core.AnthropicUsage{
				InputTokens:  c.UsageMetadata.PromptTokenCount,
				OutputTokens: c.UsageMetadata.CandidatesTokenCount,
			}
		}

		frames = append(frames, mustJSON(openAIChunk))
	}

	return frames, nil
}

// drain is intentionally empty because Gemini SSE candidate chunks are self-contained
// and carry their own finishReason without requiring buffered end-of-stream assembly.
func (t geminiToOpenAI) drain(state *core.StreamState) [][]byte {
	return nil
}

func mapGeminiFinishReason(reason string, hasToolCalls bool) string {
	switch strings.ToUpper(reason) {
	case "STOP":
		if hasToolCalls {
			return "tool_calls"
		}
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION":
		return "content_filter"
	default:
		return "stop"
	}
}
