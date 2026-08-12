package response_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/translate"
	"github.com/oniharnantyo/tinyroute/internal/translate/response"
)

func TestOpenAIToAnthropicResponseStream(t *testing.T) {
	_, respTrans, ok := translate.Lookup("anthropic", "openai")
	if !ok || respTrans == nil {
		t.Fatalf("expected anthropic->openai response translator to be registered")
	}

	t.Run("plain text stream with final usage", func(t *testing.T) {
		state := translate.NewStreamState()
		chunks := [][]byte{
			[]byte(`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"}}],"usage":null}`),
			[]byte(`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":" world!"}}],"usage":null}`),
			[]byte(`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completionTokens":5,"prompt_tokens_details":{"cached_tokens":2,"cache_creation_tokens":1}}}`),
		}

		var allFrames [][]byte
		for _, c := range chunks {
			frames, err := respTrans.TranslateResponse(c, state)
			if err != nil {
				t.Fatalf("unexpected chunk translate error: %v", err)
			}
			allFrames = append(allFrames, frames...)
		}

		// Drain sentinel (nil chunk)
		drainFrames, err := respTrans.TranslateResponse(nil, state)
		if err != nil {
			t.Fatalf("unexpected drain translate error: %v", err)
		}
		allFrames = append(allFrames, drainFrames...)

		// Verify event sequence:
		// 1. message_start
		// 2. content_block_start (text, index 0)
		// 3. content_block_delta ("Hello")
		// 4. content_block_delta (" world!")
		// 5. content_block_stop (index 0)
		// 6. message_delta (stop_reason: end_turn, usage: input 7, output 0, cached 2, creation 1)
		// 7. message_stop
		if len(allFrames) < 7 {
			t.Fatalf("expected at least 7 frames, got %d", len(allFrames))
		}

		var types []string
		for _, f := range allFrames {
			var m map[string]interface{}
			if err := json.Unmarshal(f, &m); err != nil {
				t.Fatalf("unmarshal frame failed: %v", err)
			}
			types = append(types, m["type"].(string))
		}

		expectedTypes := []string{
			"message_start",
			"content_block_start",
			"content_block_delta",
			"content_block_delta",
			"content_block_stop",
			"message_delta",
			"message_stop",
		}
		for i, exp := range expectedTypes {
			if types[i] != exp {
				t.Errorf("frame %d type mismatch: got %s, want %s", i, types[i], exp)
			}
		}

		// Check message_start ID
		var msgStart map[string]interface{}
		json.Unmarshal(allFrames[0], &msgStart)
		msgObj := msgStart["message"].(map[string]interface{})
		if !strings.HasPrefix(msgObj["id"].(string), "msg_") {
			t.Errorf("expected synthesized message ID starting with msg_, got %v", msgObj["id"])
		}

		// Check message_delta usage
		var msgDelta map[string]interface{}
		json.Unmarshal(allFrames[5], &msgDelta)
		usageObj := msgDelta["usage"].(map[string]interface{})
		if int(usageObj["input_tokens"].(float64)) != 7 { // 10 - 2 - 1 = 7
			t.Errorf("expected input_tokens 7, got %v", usageObj["input_tokens"])
		}
	})

	t.Run("reasoning content stream", func(t *testing.T) {
		state := translate.NewStreamState()
		chunks := [][]byte{
			[]byte(`{"id":"chatcmpl-2","model":"o3-mini","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"Let me think."}}],"usage":null}`),
			[]byte(`{"id":"chatcmpl-2","model":"o3-mini","choices":[{"index":0,"delta":{"content":"Answer"}}],"usage":null}`),
			[]byte(`{"id":"chatcmpl-2","model":"o3-mini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":null}`),
		}

		var allFrames [][]byte
		for _, c := range chunks {
			frames, err := respTrans.TranslateResponse(c, state)
			if err != nil {
				t.Fatalf("unexpected chunk error: %v", err)
			}
			allFrames = append(allFrames, frames...)
		}
		drainFrames, _ := respTrans.TranslateResponse(nil, state)
		allFrames = append(allFrames, drainFrames...)

		// Types should include thinking content_block_start (index 0) and text content_block_start (index 1)
		var types []string
		for _, f := range allFrames {
			var m map[string]interface{}
			json.Unmarshal(f, &m)
			types = append(types, m["type"].(string))
		}

		expectedTypes := []string{
			"message_start",
			"content_block_start", // thinking
			"content_block_delta", // thinking_delta
			"content_block_stop",  // thinking stop (triggered when text delta arrives or finish)
			"content_block_start", // text
			"content_block_delta", // text_delta
			"content_block_stop",  // text stop
			"message_delta",
			"message_stop",
		}
		for i, exp := range expectedTypes {
			if i >= len(types) || types[i] != exp {
				t.Errorf("frame %d type mismatch: got %v, want %s (all: %v)", i, types, exp, types)
				break
			}
		}
	})

	t.Run("tool call stream", func(t *testing.T) {
		state := translate.NewStreamState()
		chunks := [][]byte{
			[]byte(`{"id":"chatcmpl-3","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_99","function":{"name":"get_weather","arguments":""}}]}}],"usage":null}`),
			[]byte(`{"id":"chatcmpl-3","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"loc\":"}}]}}],"usage":null}`),
			[]byte(`{"id":"chatcmpl-3","model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"NYC\"}"}}]}}],"usage":null}`),
			[]byte(`{"id":"chatcmpl-3","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":null}`),
		}

		var allFrames [][]byte
		for _, c := range chunks {
			frames, err := respTrans.TranslateResponse(c, state)
			if err != nil {
				t.Fatalf("unexpected chunk error: %v", err)
			}
			allFrames = append(allFrames, frames...)
		}
		drainFrames, _ := respTrans.TranslateResponse(nil, state)
		allFrames = append(allFrames, drainFrames...)

		// Tool calls arguments should be buffered and emitted as a single input_json_delta at drain/finish
		var hasToolUseStart, hasInputJSONDelta, hasToolUseStop bool
		for _, f := range allFrames {
			var m map[string]interface{}
			json.Unmarshal(f, &m)
			if m["type"] == "content_block_start" {
				cb := m["content_block"].(map[string]interface{})
				if cb["type"] == "tool_use" {
					hasToolUseStart = true
				}
			}
			if m["type"] == "content_block_delta" {
				delta := m["delta"].(map[string]interface{})
				if delta["type"] == "input_json_delta" && delta["partial_json"] == `{"loc":"NYC"}` {
					hasInputJSONDelta = true
				}
			}
			if m["type"] == "content_block_stop" {
				hasToolUseStop = true
			}
		}

		if !hasToolUseStart || !hasInputJSONDelta || !hasToolUseStop {
			t.Errorf("tool call stream frames missing expected tool_use structures")
		}
	})
}

func TestNonStreamingMessageJSON(t *testing.T) {
	state := translate.NewStreamState()
	state.MessageID = "msg_test123"
	state.Model = "claude-3-5-sonnet-20241022"
	state.TextBuffer.WriteString("Hello world")
	state.FinishReason = "stop"
	state.ToolCalls[0] = core.ToolCallState{BlockIndex: 1, ID: "tool_1", Name: "get_weather"}
	state.ToolArgBuffers[0] = []byte(`{"loc":"NYC"}`)

	b := response.NonStreamingMessageJSON(state)
	var obj map[string]interface{}
	if err := json.Unmarshal(b, &obj); err != nil {
		t.Fatalf("failed to unmarshal NonStreamingMessageJSON: %v", err)
	}
	if obj["id"] != "msg_test123" {
		t.Errorf("expected id=msg_test123, got %v", obj["id"])
	}
}
