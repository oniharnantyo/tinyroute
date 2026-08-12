package request_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/translate"
	_ "github.com/oniharnantyo/tinyroute/internal/translate/request"
)

func TestOpenAIToGeminiRequest(t *testing.T) {
	reqTrans, _, ok := translate.Lookup("openai", "gemini")
	if !ok || reqTrans == nil {
		t.Fatalf("expected openai->gemini request translator to be registered")
	}

	t.Run("basic system instruction and contents mapping", func(t *testing.T) {
		input := `{
			"model": "gemini-1.5-pro",
			"messages": [
				{"role": "system", "content": "System instruction here"},
				{"role": "user", "content": "Turn 1"},
				{"role": "assistant", "content": "Turn 2"}
			],
			"temperature": 0.7,
			"top_p": 0.9,
			"top_k": 40,
			"max_tokens": 1000
		}`

		gotBytes, err := reqTrans.TranslateRequest([]byte(input), nil)
		if err != nil {
			t.Fatalf("unexpected translation error: %v", err)
		}

		var got map[string]interface{}
		json.Unmarshal(gotBytes, &got)

		// Assert systemInstruction
		sys, ok := got["systemInstruction"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected systemInstruction in payload")
		}
		sysParts := sys["parts"].([]interface{})
		if sysParts[0].(map[string]interface{})["text"] != "System instruction here" {
			t.Errorf("system text mismatch")
		}

		// Assert contents
		contents := got["contents"].([]interface{})
		if len(contents) != 2 {
			t.Fatalf("expected 2 contents turns, got %d", len(contents))
		}
		c0 := contents[0].(map[string]interface{})
		if c0["role"] != "user" {
			t.Errorf("expected user role for c0, got %s", c0["role"])
		}
		c1 := contents[1].(map[string]interface{})
		if c1["role"] != "model" {
			t.Errorf("expected model role for assistant turn, got %s", c1["role"])
		}

		// Assert generationConfig
		genConfig := got["generationConfig"].(map[string]interface{})
		if genConfig["topP"].(float64) != 0.9 || genConfig["topK"].(float64) != 40 || genConfig["maxOutputTokens"].(float64) != 1000 {
			t.Errorf("generationConfig mismatch: %+v", genConfig)
		}
	})

	t.Run("adjacent same-role turn merging", func(t *testing.T) {
		input := `{
			"messages": [
				{"role": "user", "content": "Hello"},
				{"role": "user", "content": "More text"}
			]
		}`

		gotBytes, err := reqTrans.TranslateRequest([]byte(input), nil)
		if err != nil {
			t.Fatalf("unexpected translation error: %v", err)
		}

		var got map[string]interface{}
		json.Unmarshal(gotBytes, &got)

		contents := got["contents"].([]interface{})
		if len(contents) != 1 {
			t.Fatalf("expected adjacent user turns to be merged into 1 turn, got %d", len(contents))
		}
		c0 := contents[0].(map[string]interface{})
		parts := c0["parts"].([]interface{})
		if len(parts) != 2 {
			t.Fatalf("expected 2 parts in merged user turn, got %d", len(parts))
		}
	})

	t.Run("tools and function calls with name sanitization", func(t *testing.T) {
		input := `{
			"messages": [
				{"role": "user", "content": "Call tool"},
				{
					"role": "assistant",
					"tool_calls": [
						{
							"id": "call_abc",
							"type": "function",
							"function": {"name": "get-weather-v1!", "arguments": "{\"location\":\"NYC\"}"}
						}
					]
				},
				{
					"role": "tool",
					"tool_call_id": "call_abc",
					"content": "72F"
				}
			],
			"tools": [
				{
					"type": "function",
					"function": {
						"name": "get-weather-v1!",
						"description": "Get weather",
						"parameters": {"type": "object"}
					}
				}
			]
		}`

		gotBytes, err := reqTrans.TranslateRequest([]byte(input), nil)
		if err != nil {
			t.Fatalf("unexpected translation error: %v", err)
		}

		var got map[string]interface{}
		json.Unmarshal(gotBytes, &got)

		// Tools declaration check
		tools := got["tools"].([]interface{})
		t0 := tools[0].(map[string]interface{})
		decls := t0["functionDeclarations"].([]interface{})
		decl0 := decls[0].(map[string]interface{})
		if decl0["name"] != "get-weather-v1_" {
			t.Errorf("expected sanitized tool name get-weather-v1_, got %s", decl0["name"])
		}

		// Function call & function response check
		contents := got["contents"].([]interface{})
		if len(contents) != 3 {
			t.Fatalf("expected 3 contents turns, got %d", len(contents))
		}

		// Assistant turn functionCall
		c1Parts := contents[1].(map[string]interface{})["parts"].([]interface{})
		fc := c1Parts[0].(map[string]interface{})["functionCall"].(map[string]interface{})
		if fc["name"] != "get-weather-v1_" {
			t.Errorf("functionCall name = %s, want get-weather-v1_", fc["name"])
		}

		// Tool result functionResponse with {result} wrap
		c2Parts := contents[2].(map[string]interface{})["parts"].([]interface{})
		fr := c2Parts[0].(map[string]interface{})["functionResponse"].(map[string]interface{})
		if fr["name"] != "get-weather-v1_" {
			t.Errorf("functionResponse name = %s, want get-weather-v1_", fr["name"])
		}
		respMap := fr["response"].(map[string]interface{})
		if !reflect.DeepEqual(respMap, map[string]interface{}{"result": "72F"}) {
			t.Errorf("functionResponse response wrap mismatch: %+v", respMap)
		}
	})

	t.Run("thoughtSignature placeholder on reasoning and tool parts", func(t *testing.T) {
		input := `{
			"messages": [
				{
					"role": "assistant",
					"reasoning_content": "Thinking process here...",
					"tool_calls": [
						{
							"id": "call_abc",
							"type": "function",
							"function": {"name": "get_weather", "arguments": "{}"}
						}
					]
				}
			]
		}`

		gotBytes, err := reqTrans.TranslateRequest([]byte(input), nil)
		if err != nil {
			t.Fatalf("unexpected translation error: %v", err)
		}

		var got map[string]interface{}
		json.Unmarshal(gotBytes, &got)

		contents := got["contents"].([]interface{})
		c0 := contents[0].(map[string]interface{})
		parts := c0["parts"].([]interface{})
		if len(parts) != 2 {
			t.Fatalf("expected 2 parts (thought + functionCall), got %d", len(parts))
		}

		p0 := parts[0].(map[string]interface{})
		if p0["thought"] != true || p0["thoughtSignature"] == "" {
			t.Errorf("expected thought=true and non-empty thoughtSignature on reasoning part: %+v", p0)
		}

		p1 := parts[1].(map[string]interface{})
		if p1["functionCall"] == nil || p1["thoughtSignature"] == "" {
			t.Errorf("expected functionCall and non-empty thoughtSignature on tool part: %+v", p1)
		}
	})

	t.Run("image_url parts mapping", func(t *testing.T) {
		input := `{
			"messages": [
				{
					"role": "user",
					"content": [
						{"type": "text", "text": "What is this image?"},
						{"type": "image_url", "image_url": {"url": "data:image/png;base64,iVBORw0KGgo="}}
					]
				}
			]
		}`

		gotBytes, err := reqTrans.TranslateRequest([]byte(input), nil)
		if err != nil {
			t.Fatalf("unexpected translation error: %v", err)
		}

		var got map[string]interface{}
		json.Unmarshal(gotBytes, &got)
		contents := got["contents"].([]interface{})
		c0 := contents[0].(map[string]interface{})
		parts := c0["parts"].([]interface{})
		if len(parts) != 2 {
			t.Fatalf("expected 2 parts (text + inlineData), got %d", len(parts))
		}
		inlineData := parts[1].(map[string]interface{})["inlineData"].(map[string]interface{})
		if inlineData["mimeType"] != "image/png" || inlineData["data"] != "iVBORw0KGgo=" {
			t.Errorf("inlineData mismatch: %+v", inlineData)
		}
	})
}
