// Package request implements request-body translators. It ports 9router's
// translator/request/claude-to-openai.js into Go.
package request

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/translate"
)

func init() {
	translate.Register("anthropic", "openai", anthropicToOpenAI{}, nil)
}

// anthropicToOpenAI converts Anthropic Messages request bodies into OpenAI
// Chat Completions bodies (see design §6 request mapping).
type anthropicToOpenAI struct{}

// contentBlock is one element of an Anthropic message content array.
type contentBlock struct {
	Type      string           `json:"type"`
	Text      string           `json:"text"`
	Source    *anThropicSource `json:"source"`
	URL       string           `json:"url"`
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Input     json.RawMessage  `json:"input"`
	ToolUseID string           `json:"tool_use_id"`
	Content   json.RawMessage  `json:"content"`
}

type anThropicSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
	URL       string `json:"url"`
}

type anThropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anThropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anThropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// TranslateRequest implements core.RequestTranslator.
func (anthropicToOpenAI) TranslateRequest(body []byte, state *core.StreamState) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("anthropic→openai: parse request: %w", err)
	}

	var messages []anThropicMessage
	if m, ok := raw["messages"]; ok {
		if err := json.Unmarshal(m, &messages); err != nil {
			return nil, fmt.Errorf("anthropic→openai: parse messages: %w", err)
		}
	}

	openAIMessages, toolCallIDs, err := convertMessages(messages, systemText(raw["system"]))
	if err != nil {
		return nil, err
	}

	// Synthesize a "[No response received]" stub for every assistant tool call
	// that lacks a matching tool response: OpenAI rejects requests whose
	// tool_call_ids are referenced without a following role:"tool" message.
	for id := range toolCallIDs {
		openAIMessages = append(openAIMessages, map[string]interface{}{
			"role":         "tool",
			"tool_call_id": id,
			"content":      "[No response received]",
		})
	}

	msgsJSON, err := json.Marshal(openAIMessages)
	if err != nil {
		return nil, err
	}
	raw["messages"] = msgsJSON

	// Top-level system is now folded into the message list; drop the field.
	delete(raw, "system")

	if toolsRaw, ok := raw["tools"]; ok {
		var tools []anThropicTool
		if err := json.Unmarshal(toolsRaw, &tools); err != nil {
			return nil, fmt.Errorf("anthropic→openai: parse tools: %w", err)
		}
		openAITools := make([]map[string]interface{}, 0, len(tools))
		for _, t := range tools {
			if len(t.InputSchema) == 0 || bytes.Equal(bytes.TrimSpace(t.InputSchema), []byte("null")) {
				continue
			}
			fn := map[string]interface{}{"name": t.Name}
			if t.Description != "" {
				fn["description"] = t.Description
			}
			fn["parameters"] = json.RawMessage(t.InputSchema)
			openAITools = append(openAITools, map[string]interface{}{
				"type":     "function",
				"function": fn,
			})
		}
		if len(openAITools) > 0 {
			toolsJSON, err := json.Marshal(openAITools)
			if err != nil {
				return nil, err
			}
			raw["tools"] = toolsJSON
		} else {
			delete(raw, "tools")
		}
	}

	if tcRaw, ok := raw["tool_choice"]; ok {
		var tc anThropicToolChoice
		if err := json.Unmarshal(tcRaw, &tc); err == nil {
			var mapped interface{}
			switch tc.Type {
			case "any":
				mapped = "required"
			case "auto":
				mapped = "auto"
			case "tool":
				mapped = map[string]interface{}{
					"type":     "function",
					"function": map[string]interface{}{"name": tc.Name},
				}
			default:
				mapped = "auto"
			}
			mappedJSON, err := json.Marshal(mapped)
			if err != nil {
				return nil, err
			}
			raw["tool_choice"] = mappedJSON
		}
	}

	return json.Marshal(raw)
}

// convertMessages converts the Anthropic message list into an OpenAI message
// list. It returns the messages and the set of assistant tool_call ids that
// have no matching role:"tool" response (candidates for stub insertion).
func convertMessages(msgs []anThropicMessage, system string) ([]map[string]interface{}, map[string]bool, error) {
	out := make([]map[string]interface{}, 0, len(msgs)+1)
	if system != "" {
		out = append(out, map[string]interface{}{"role": "system", "content": system})
	}

	allToolIDs := map[string]bool{}
	responded := map[string]bool{}

	for _, m := range msgs {
		role := m.Role
		// Mid-conversation system messages are not allowed by OpenAI; wrap them
		// as a user message inside <instructions>.
		if role == "system" {
			out = append(out, map[string]interface{}{"role": "user", "content": "<instructions>" + messageText(m.Content) + "</instructions>"})
			continue
		}

		blocks := parseBlocks(m.Content)
		if len(blocks) == 0 {
			out = append(out, map[string]interface{}{"role": role, "content": ""})
			continue
		}

		var textParts []string
		var contentParts []map[string]interface{}
		var toolCalls []map[string]interface{}
		hasImage := false

		for _, b := range blocks {
			switch b.Type {
			case "", "text":
				if b.Text != "" {
					textParts = append(textParts, b.Text)
					contentParts = append(contentParts, map[string]interface{}{
						"type": "text",
						"text": b.Text,
					})
				}
			case "image":
				if url := imageToDataURL(b); url != "" {
					hasImage = true
					contentParts = append(contentParts, map[string]interface{}{
						"type":      "image_url",
						"image_url": map[string]interface{}{"url": url},
					})
				}
			case "tool_use":
				argsJSON := "{}"
				if len(b.Input) > 0 && !bytes.Equal(bytes.TrimSpace(b.Input), []byte("null")) {
					argsJSON = string(b.Input)
				}
				toolCalls = append(toolCalls, map[string]interface{}{
					"id":   b.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      b.Name,
						"arguments": argsJSON,
					},
				})
				if b.ID != "" {
					allToolIDs[b.ID] = true
				}
			case "tool_result":
				id, err := toolResultID(b)
				if err != nil {
					return nil, nil, err
				}
				responded[id] = true
				out = append(out, map[string]interface{}{
					"role":         "tool",
					"tool_call_id": id,
					"content":      renderToolResult(b.Content),
				})
			}
		}

		if role == "assistant" && len(toolCalls) > 0 {
			msg := map[string]interface{}{"role": "assistant", "content": nil, "tool_calls": toolCalls}
			if hasImage {
				msg["content"] = contentParts
			} else if len(textParts) > 0 {
				msg["content"] = strings.Join(textParts, "\n")
			}
			out = append(out, msg)
		} else if hasImage {
			out = append(out, map[string]interface{}{"role": role, "content": contentParts})
		} else if len(textParts) > 0 || len(blocks) == 0 {
			out = append(out, map[string]interface{}{"role": role, "content": strings.Join(textParts, "\n")})
		}
	}

	missing := map[string]bool{}
	for id := range allToolIDs {
		if !responded[id] {
			missing[id] = true
		}
	}
	return out, missing, nil
}

// systemText joins the anthropic top-level system field (string or block array)
// into a plain string, stripping provider billing artifacts.
func systemText(sys json.RawMessage) string {
	if len(sys) == 0 || bytes.Equal(bytes.TrimSpace(sys), []byte("null")) {
		return ""
	}
	var s string
	if json.Unmarshal(sys, &s) == nil {
		return stripBilling(s)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(sys, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" || b.Type == "" {
				parts = append(parts, b.Text)
			}
		}
		return stripBilling(strings.Join(parts, "\n"))
	}
	return ""
}

// stripBilling removes the anthropic billing-header artifact from a prompt.
func stripBilling(s string) string {
	return strings.ReplaceAll(s, "x-anthropic-billing-header:", " ")
}

func parseBlocks(raw json.RawMessage) []contentBlock {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if s == "" {
			return nil
		}
		return []contentBlock{{Type: "text", Text: s}}
	}
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		return blocks
	}
	return nil
}

func messageText(raw json.RawMessage) string {
	blocks := parseBlocks(raw)
	var parts []string
	for _, b := range blocks {
		if b.Type == "" || b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func imageToDataURL(b contentBlock) string {
	if b.URL != "" {
		return b.URL
	}
	if b.Source != nil && b.Source.Type == "base64" {
		mime := b.Source.MediaType
		if mime == "" {
			mime = "image/png"
		}
		return fmt.Sprintf("data:%s;base64,%s", mime, b.Source.Data)
	}
	return ""
}

func toolResultID(b contentBlock) (string, error) {
	if b.ToolUseID != "" {
		return b.ToolUseID, nil
	}
	var obj struct {
		ToolUseID string `json:"tool_use_id"`
	}
	if len(b.Content) > 0 && json.Unmarshal(b.Content, &obj) == nil && obj.ToolUseID != "" {
		return obj.ToolUseID, nil
	}
	return "", fmt.Errorf("anthropic→openai: tool_result missing tool_use_id")
}

func renderToolResult(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "" || b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}
