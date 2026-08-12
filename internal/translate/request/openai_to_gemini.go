// Package request implements request-body translators.
package request

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/translate"
	"github.com/oniharnantyo/tinyroute/internal/translate/concerns"
)

const thoughtSignaturePlaceholder = "EuwGCukGAXLI2nxwZIq54WWSoL/YN0P3TsDZ7zRnLi8g0S4aVr2HUGxvaHKySuY6HAVzcE0GPGjXrytLIldxthSvfxgUlJh6Qa9Z+Oj5QZBlYdg6HaJ6yuY5R7waE6rdwBsRf7Ft2j3DJ9rMi9qhWFqApewYtPhls3VHtuvND3l8Rm09+lbAXQs6KKWEWrxNLKTBkfpMgXhRERc/TQRMZu1twAablm6/Zk1tsYRvfWKLsNbeKF+CCojJdXJKvnR/8Ouuoa+Y2Ti20hcW7aZIIjZDFYPU//k6Ybmhg69J/imbFai2ckhfLaisqdDkdoIiBJScTOUvYqP6AE9d4MsydSC+UlhIMk4hoP76R8vUSCZRMkjOaDXstf/QoVZKbt94wyRZgAJ1G0BqI8L5ow86kLpA4wJEtxsRGymOE4bKUvApveBakYDNM9APkf+LbtbzWSseGjoZcSlycF9iN8Q2XNYKRrHbv3Lr5Y8JjdH/5y/6SHkNehTEZugaeGnSPSyCTWto1kQgHpxdWmhkLfJGNUGLmue7Mesj4TSms4J33mRpYVhNB/J333FCqIP0hr/E7BkkjEn7yZ4X7SQlh+xKPurapsnHRwiKmtsilmEFrnTE9iQr+pMr6M29qqFNv1tr5yumbaJw8JW9sB15tNsRv+dW6BjNanbsKz7HCgKUBc8tGy+7YuhXzAfViyRefcjK7eZW0Fbyt7AbybJTKz78W8NH7ye6LAwzOebXpeZ4D43fNIt8bKh26qgduSQv/7o+pAflkuqHZ99YWgHQ8h8OkZFi3eOiSYjsjhdZ/czWOdoPI/OnqIldzMPF5YlrKBLFX8VhRKVmqgsmWf5PHGulHhMkVlS+XG2UIseGy69ARa93D78Gsa+1n1kJr7EEB7Rh+27vUMxVYLdz1yMSvE5nalTAlg/ZeG8+XQ0cHuAI3KbQpHW2Q++RdXfm5JzD5WdJZUU+Zn8t8UUn85BH4RxZLeE0qJikgSsKoYVBc6YhiMjhPgkR95ReimY4Z0xCJdRo1gjexOFeODZMpQF6Yxnoic7IrdgsFA3iePTbFnPp3IAM1fAThWhXJUn3QInUOTd5o1qmTmn6REbL15g/JQNl+dqUoPkhleeb2V3kjqp1okmO3wMZbPknR3S1LZNmlS72/iBQUm+n2b/RCn4PjmM2"

func init() {
	translate.Register("openai", "gemini", openaiToGemini{}, nil)
}

// openaiToGemini converts OpenAI chat completion requests into Gemini generateContent bodies.
type openaiToGemini struct{}

type openAIReqMsg struct {
	Role             string           `json:"role"`
	Content          json.RawMessage  `json:"content"`
	ToolCalls        []openAIToolCall `json:"tool_calls"`
	ToolCallID       string           `json:"tool_call_id"`
	ReasoningContent string           `json:"reasoning_content"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	InlineData       *geminiInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
	Thought          bool                    `json:"thought,omitempty"`
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type geminiFunctionResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiSystemInstruction struct {
	Parts []geminiPart `json:"parts"`
}

type geminiFunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations"`
}

type geminiGenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	TopK            *int     `json:"topK,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
}

func (openaiToGemini) TranslateRequest(body []byte, state *core.StreamState) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("openai→gemini: parse request: %w", err)
	}

	var nameMap interface {
		Sanitize(string) string
		Restore(string) string
	}
	if state != nil {
		if state.GeminiNameMap == nil {
			state.GeminiNameMap = concerns.NewGeminiNameMap()
		}
		nameMap = state.GeminiNameMap
	} else {
		nameMap = concerns.NewGeminiNameMap()
	}

	toolCallNames := make(map[string]string) // toolCallID -> original tool name

	var msgs []openAIReqMsg
	if mRaw, ok := raw["messages"]; ok {
		if err := json.Unmarshal(mRaw, &msgs); err != nil {
			return nil, fmt.Errorf("openai→gemini: parse messages: %w", err)
		}
	}

	var contents []geminiContent
	var systemParts []geminiPart

	for _, m := range msgs {
		if m.Role == "system" {
			txt := parseMessageText(m.Content)
			if txt != "" {
				systemParts = append(systemParts, geminiPart{Text: txt})
			}
			continue
		}

		role := m.Role
		if role == "assistant" {
			role = "model"
		}

		var parts []geminiPart

		if m.Role == "tool" {
			role = "user"
			origName := toolCallNames[m.ToolCallID]
			if origName == "" {
				origName = "unknown_tool"
			}
			sanName := nameMap.Sanitize(origName)

			rawTxt := parseMessageText(m.Content)
			var respObj map[string]interface{}
			if strings.TrimSpace(rawTxt) != "" {
				_ = json.Unmarshal([]byte(rawTxt), &respObj)
			}
			if respObj == nil {
				respObj = map[string]interface{}{"result": rawTxt}
			}

			parts = append(parts, geminiPart{
				FunctionResponse: &geminiFunctionResponse{
					Name:     sanName,
					Response: respObj,
				},
			})
		} else {
			// If message has reasoning_content
			if m.ReasoningContent != "" {
				parts = append(parts, geminiPart{
					Text:             m.ReasoningContent,
					Thought:          true,
					ThoughtSignature: thoughtSignaturePlaceholder,
				})
			}

			// Main text/image content
			txtParts, imgParts := parseMessageParts(m.Content)
			for _, t := range txtParts {
				parts = append(parts, geminiPart{Text: t})
			}
			for _, img := range imgParts {
				parts = append(parts, geminiPart{InlineData: img})
			}

			// Tool calls
			for _, tc := range m.ToolCalls {
				origName := tc.Function.Name
				sanName := nameMap.Sanitize(origName)
				if tc.ID != "" {
					toolCallNames[tc.ID] = origName
				}

				var argsObj map[string]interface{}
				if strings.TrimSpace(tc.Function.Arguments) != "" {
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &argsObj)
				}
				if argsObj == nil {
					argsObj = make(map[string]interface{})
				}

				parts = append(parts, geminiPart{
					FunctionCall: &geminiFunctionCall{
						Name: sanName,
						Args: argsObj,
					},
					ThoughtSignature: thoughtSignaturePlaceholder,
				})
			}
		}

		if len(parts) > 0 {
			contents = append(contents, geminiContent{
				Role:  role,
				Parts: parts,
			})
		}
	}

	contents = normalizeGeminiContents(contents)

	geminiReq := make(map[string]interface{})
	if len(contents) > 0 {
		geminiReq["contents"] = contents
	}
	if len(systemParts) > 0 {
		geminiReq["systemInstruction"] = geminiSystemInstruction{Parts: systemParts}
	}

	// Tools
	if toolsRaw, ok := raw["tools"]; ok {
		var openAITools []struct {
			Type     string `json:"type"`
			Function struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"`
			} `json:"function"`
		}
		if json.Unmarshal(toolsRaw, &openAITools) == nil {
			var decls []geminiFunctionDeclaration
			for _, t := range openAITools {
				sanName := nameMap.Sanitize(t.Function.Name)
				decls = append(decls, geminiFunctionDeclaration{
					Name:        sanName,
					Description: t.Function.Description,
					Parameters:  t.Function.Parameters,
				})
			}
			if len(decls) > 0 {
				geminiReq["tools"] = []geminiTool{{FunctionDeclarations: decls}}
			}
		}
	}

	// GenerationConfig
	genConfig := map[string]interface{}{}
	if v, ok := raw["temperature"]; ok {
		genConfig["temperature"] = v
	}
	if v, ok := raw["top_p"]; ok {
		genConfig["topP"] = v
	}
	if v, ok := raw["top_k"]; ok {
		genConfig["topK"] = v
	}
	if v, ok := raw["max_tokens"]; ok {
		genConfig["maxOutputTokens"] = v
	}
	if len(genConfig) > 0 {
		geminiReq["generationConfig"] = genConfig
	}

	return json.Marshal(geminiReq)
}

// normalizeGeminiContents merges consecutive turns with the same role.
func normalizeGeminiContents(contents []geminiContent) []geminiContent {
	if len(contents) <= 1 {
		return contents
	}
	var out []geminiContent
	for _, c := range contents {
		if len(out) > 0 && out[len(out)-1].Role == c.Role {
			out[len(out)-1].Parts = append(out[len(out)-1].Parts, c.Parts...)
		} else {
			out = append(out, c)
		}
	}
	return out
}

func parseMessageText(raw json.RawMessage) string {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	txtParts, _ := parseMessageParts(raw)
	return strings.Join(txtParts, "\n")
}

func parseMessageParts(raw json.RawMessage) ([]string, []*geminiInlineData) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if s == "" {
			return nil, nil
		}
		return []string{s}, nil
	}

	var blocks []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var texts []string
		var imgs []*geminiInlineData
		for _, b := range blocks {
			if (b.Type == "text" || b.Type == "") && b.Text != "" {
				texts = append(texts, b.Text)
			}
			if b.Type == "image_url" && b.ImageURL != nil && b.ImageURL.URL != "" {
				if inline := parseDataURI(b.ImageURL.URL); inline != nil {
					imgs = append(imgs, inline)
				}
			}
		}
		return texts, imgs
	}
	return nil, nil
}

func parseDataURI(dataURI string) *geminiInlineData {
	if !strings.HasPrefix(dataURI, "data:") {
		return nil
	}
	parts := strings.SplitN(dataURI[5:], ",", 2)
	if len(parts) != 2 {
		return nil
	}
	meta := parts[0]
	data := parts[1]

	mime := "image/png"
	metaParts := strings.Split(meta, ";")
	if len(metaParts) > 0 && metaParts[0] != "" {
		mime = metaParts[0]
	}

	if !strings.Contains(meta, "base64") {
		data = base64.StdEncoding.EncodeToString([]byte(data))
	}

	return &geminiInlineData{
		MimeType: mime,
		Data:     data,
	}
}
