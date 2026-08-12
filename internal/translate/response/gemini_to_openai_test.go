package response_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/translate"
	_ "github.com/oniharnantyo/tinyroute/internal/translate/request"
	_ "github.com/oniharnantyo/tinyroute/internal/translate/response"
)

func TestGeminiToOpenAIResponseStream(t *testing.T) {
	_, respTrans, ok := translate.Lookup("openai", "gemini")
	if !ok || respTrans == nil {
		t.Fatalf("expected openai->gemini response translator to be registered")
	}

	t.Run("text, thought, and usage metadata stream", func(t *testing.T) {
		state := translate.NewStreamState()
		chunks := [][]byte{
			[]byte(`{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"Let me think carefully.","thought":true}]}}]}`),
			[]byte(`{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"The answer is 42."}]}}]}`),
			[]byte(`{"candidates":[{"index":0,"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":15,"candidatesTokenCount":10,"totalTokenCount":25}}`),
		}

		var allFrames [][]byte
		for _, c := range chunks {
			frames, err := respTrans.TranslateResponse(c, state)
			if err != nil {
				t.Fatalf("unexpected translation error: %v", err)
			}
			allFrames = append(allFrames, frames...)
		}

		drainFrames, _ := respTrans.TranslateResponse(nil, state)
		allFrames = append(allFrames, drainFrames...)

		if len(allFrames) < 3 {
			t.Fatalf("expected at least 3 frames, got %d", len(allFrames))
		}

		// Frame 0: reasoning_content
		var f0 map[string]interface{}
		json.Unmarshal(allFrames[0], &f0)
		delta0 := f0["choices"].([]interface{})[0].(map[string]interface{})["delta"].(map[string]interface{})
		if delta0["reasoning_content"] != "Let me think carefully." {
			t.Errorf("reasoning_content = %v, want Let me think carefully.", delta0["reasoning_content"])
		}

		// Frame 1: text content
		var f1 map[string]interface{}
		json.Unmarshal(allFrames[1], &f1)
		delta1 := f1["choices"].([]interface{})[0].(map[string]interface{})["delta"].(map[string]interface{})
		if delta1["content"] != "The answer is 42." {
			t.Errorf("content = %v, want The answer is 42.", delta1["content"])
		}

		// Frame 2: finishReason STOP -> stop & usage
		var f2 map[string]interface{}
		json.Unmarshal(allFrames[2], &f2)
		choice2 := f2["choices"].([]interface{})[0].(map[string]interface{})
		if choice2["finish_reason"] != "stop" {
			t.Errorf("finish_reason = %v, want stop", choice2["finish_reason"])
		}
		usage2 := f2["usage"].(map[string]interface{})
		if usage2["prompt_tokens"].(float64) != 15 || usage2["completion_tokens"].(float64) != 10 {
			t.Errorf("usage = %+v, want prompt 15 completion 10", usage2)
		}
	})

	t.Run("functionCall finishReason STOP override to tool_calls", func(t *testing.T) {
		state := translate.NewStreamState()
		chunk := []byte(`{
			"candidates": [
				{
					"index": 0,
					"content": {
						"role": "model",
						"parts": [
							{
								"functionCall": {
									"name": "get_weather",
									"args": {"loc": "London"}
								}
							}
						]
					},
					"finishReason": "STOP"
				}
			]
		}`)

		frames, err := respTrans.TranslateResponse(chunk, state)
		if err != nil {
			t.Fatalf("unexpected chunk translate error: %v", err)
		}

		if len(frames) != 1 {
			t.Fatalf("expected 1 frame, got %d", len(frames))
		}

		var f map[string]interface{}
		json.Unmarshal(frames[0], &f)
		choice := f["choices"].([]interface{})[0].(map[string]interface{})

		if choice["finish_reason"] != "tool_calls" {
			t.Errorf("finish_reason = %v, want tool_calls override when functionCall present", choice["finish_reason"])
		}

		delta := choice["delta"].(map[string]interface{})
		toolCalls := delta["tool_calls"].([]interface{})
		if len(toolCalls) != 1 {
			t.Fatalf("expected 1 tool_call, got %d", len(toolCalls))
		}
		fn := toolCalls[0].(map[string]interface{})["function"].(map[string]interface{})
		if fn["name"] != "get_weather" || fn["arguments"] != `{"loc":"London"}` {
			t.Errorf("tool_call function mismatch: %+v", fn)
		}
	})

	t.Run("finishReason mappings", func(t *testing.T) {
		for _, tc := range []struct {
			reason string
			want   string
		}{
			{"MAX_TOKENS", "length"},
			{"SAFETY", "content_filter"},
			{"RECITATION", "content_filter"},
			{"UNKNOWN_REASON", "stop"},
		} {
			state := translate.NewStreamState()
			chunk := []byte(`{"candidates":[{"index":0,"finishReason":"` + tc.reason + `"}]}`)
			frames, err := respTrans.TranslateResponse(chunk, state)
			if err != nil || len(frames) != 1 {
				t.Fatalf("failed for %s: %v", tc.reason, err)
			}
			var f map[string]interface{}
			json.Unmarshal(frames[0], &f)
			choice := f["choices"].([]interface{})[0].(map[string]interface{})
			if choice["finish_reason"] != tc.want {
				t.Errorf("reason %s: got %v, want %s", tc.reason, choice["finish_reason"], tc.want)
			}
		}
	})

	t.Run("function name sanitization and restoral round-trip via StreamState", func(t *testing.T) {
		reqTrans, respTrans, ok := translate.Lookup("openai", "gemini")
		if !ok {
			t.Fatalf("lookup failed")
		}

		state := translate.NewStreamState()
		reqBody := `{
			"messages": [
				{"role": "user", "content": "Fetch data"},
				{
					"role": "assistant",
					"tool_calls": [
						{
							"id": "call_1",
							"type": "function",
							"function": {"name": "get-weather-v1!", "arguments": "{}"}
						}
					]
				}
			]
		}`

		xReq, err := reqTrans.TranslateRequest([]byte(reqBody), state)
		if err != nil {
			t.Fatalf("TranslateRequest failed: %v", err)
		}

		if !bytes.Contains(xReq, []byte("get-weather-v1_")) {
			t.Fatalf("expected sanitized name 'get-weather-v1_' in translated request body, got: %s", string(xReq))
		}

		geminiSSEChunk := `{
			"candidates": [
				{
					"content": {
						"parts": [
							{
								"functionCall": {
									"name": "get-weather-v1_",
									"args": {"loc": "NYC"}
								}
							}
						],
						"role": "model"
					},
					"finishReason": "STOP",
					"index": 0
				}
			]
		}`

		frames, err := respTrans.TranslateResponse([]byte(geminiSSEChunk), state)
		if err != nil {
			t.Fatalf("TranslateResponse failed: %v", err)
		}
		if len(frames) != 1 {
			t.Fatalf("expected 1 frame, got %d", len(frames))
		}

		if !bytes.Contains(frames[0], []byte("get-weather-v1!")) {
			t.Errorf("expected restored tool name 'get-weather-v1!' in response frame, got: %s", string(frames[0]))
		}
	})
}
