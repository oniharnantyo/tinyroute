package request_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/translate"
	_ "github.com/oniharnantyo/tinyroute/internal/translate/request"
)

func TestAnthropicToOpenAIRequest(t *testing.T) {
	reqTrans, _, ok := translate.Lookup("anthropic", "openai")
	if !ok || reqTrans == nil {
		t.Fatalf("expected anthropic->openai request translator to be registered")
	}

	tests := []struct {
		name     string
		input    string
		expected map[string]interface{}
	}{
		{
			name: "system string and simple user message",
			input: `{
				"model": "claude-3-5-sonnet-20241022",
				"system": "You are a helpful assistant.",
				"messages": [
					{"role": "user", "content": "Hello!"}
				]
			}`,
			expected: map[string]interface{}{
				"model": "claude-3-5-sonnet-20241022",
				"messages": []interface{}{
					map[string]interface{}{"role": "system", "content": "You are a helpful assistant."},
					map[string]interface{}{"role": "user", "content": "Hello!"},
				},
			},
		},
		{
			name: "system block array with billing header strip",
			input: `{
				"model": "claude-3-5-sonnet-20241022",
				"system": [
					{"type": "text", "text": "x-anthropic-billing-header: System prompt part 1\n"},
					{"type": "text", "text": "System prompt part 2"}
				],
				"messages": [
					{"role": "user", "content": "Hi"}
				]
			}`,
			expected: map[string]interface{}{
				"model": "claude-3-5-sonnet-20241022",
				"messages": []interface{}{
					map[string]interface{}{"role": "system", "content": "  System prompt part 1\n\nSystem prompt part 2"},
					map[string]interface{}{"role": "user", "content": "Hi"},
				},
			},
		},
		{
			name: "mid-conversation system message and image base64",
			input: `{
				"model": "claude-3-5-sonnet-20241022",
				"messages": [
					{"role": "user", "content": "Hi"},
					{"role": "system", "content": "Important context"},
					{"role": "user", "content": [
						{"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "iVBORw0KGgo"}}
					]}
				]
			}`,
			expected: map[string]interface{}{
				"model": "claude-3-5-sonnet-20241022",
				"messages": []interface{}{
					map[string]interface{}{"role": "user", "content": "Hi"},
					map[string]interface{}{"role": "user", "content": "<instructions>Important context</instructions>"},
					map[string]interface{}{
						"role": "user",
						"content": []interface{}{
							map[string]interface{}{
								"type": "image_url",
								"image_url": map[string]interface{}{
									"url": "data:image/png;base64,iVBORw0KGgo",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "tools and tool choice any",
			input: `{
				"model": "claude-3-5-sonnet-20241022",
				"tools": [
					{
						"name": "get_weather",
						"description": "Get current weather",
						"input_schema": {"type": "object", "properties": {"location": {"type": "string"}}}
					}
				],
				"tool_choice": {"type": "any"},
				"messages": [
					{"role": "user", "content": "Weather in London?"}
				]
			}`,
			expected: map[string]interface{}{
				"model": "claude-3-5-sonnet-20241022",
				"tools": []interface{}{
					map[string]interface{}{
						"type": "function",
						"function": map[string]interface{}{
							"name":        "get_weather",
							"description": "Get current weather",
							"parameters":  map[string]interface{}{"type": "object", "properties": map[string]interface{}{"location": map[string]interface{}{"type": "string"}}},
						},
					},
				},
				"tool_choice": "required",
				"messages": []interface{}{
					map[string]interface{}{"role": "user", "content": "Weather in London?"},
				},
			},
		},
		{
			name: "tool_use to tool_calls and tool_result and missing stub insertion",
			input: `{
				"model": "claude-3-5-sonnet-20241022",
				"messages": [
					{"role": "user", "content": "Check weather and stock"},
					{
						"role": "assistant",
						"content": [
							{"type": "tool_use", "id": "call_1", "name": "get_weather", "input": {"loc": "NYC"}},
							{"type": "tool_use", "id": "call_2", "name": "get_stock", "input": {"ticker": "AAPL"}}
						]
					},
					{
						"role": "user",
						"content": [
							{"type": "tool_result", "tool_use_id": "call_1", "content": "72F and Sunny"}
						]
					}
				]
			}`,
			expected: map[string]interface{}{
				"model": "claude-3-5-sonnet-20241022",
				"messages": []interface{}{
					map[string]interface{}{"role": "user", "content": "Check weather and stock"},
					map[string]interface{}{
						"role":    "assistant",
						"content": nil,
						"tool_calls": []interface{}{
							map[string]interface{}{
								"id":   "call_1",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "get_weather",
									"arguments": `{"loc": "NYC"}`,
								},
							},
							map[string]interface{}{
								"id":   "call_2",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "get_stock",
									"arguments": `{"ticker": "AAPL"}`,
								},
							},
						},
					},
					map[string]interface{}{
						"role":         "tool",
						"tool_call_id": "call_1",
						"content":      "72F and Sunny",
					},
					map[string]interface{}{
						"role":         "tool",
						"tool_call_id": "call_2",
						"content":      "[No response received]",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resBytes, err := reqTrans.TranslateRequest([]byte(tt.input), nil)
			if err != nil {
				t.Fatalf("unexpected translation error: %v", err)
			}

			var got map[string]interface{}
			if err := json.Unmarshal(resBytes, &got); err != nil {
				t.Fatalf("unmarshal translated result failed: %v", err)
			}

			expBytes, _ := json.Marshal(tt.expected)
			var expectedNormalized map[string]interface{}
			json.Unmarshal(expBytes, &expectedNormalized)

			if !reflect.DeepEqual(got, expectedNormalized) {
				gotStr, _ := json.MarshalIndent(got, "", "  ")
				expStr, _ := json.MarshalIndent(expectedNormalized, "", "  ")
				t.Errorf("translation mismatch:\nGOT:\n%s\nWANT:\n%s", string(gotStr), string(expStr))
			}
		})
	}
}
