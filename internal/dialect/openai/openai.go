package openai

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/credential"
	"github.com/oniharnantyo/tinyroute/internal/dialect"
)

func init() {
	dialect.Register(&Dialect{})
}

// Dialect implements core.Dialect for the OpenAI Chat Completions API.
type Dialect struct{}

func (d *Dialect) Name() string { return "openai" }

func (d *Dialect) Paths() []string { return []string{"/v1/chat/completions"} }

func (d *Dialect) MountPaths() []string { return []string{"/openai/v1/chat/completions"} }

// ModelsMountPath returns the inbound path for OpenAI-shaped model discovery.
func (d *Dialect) ModelsMountPath() string { return "/openai/v1/models" }

func (d *Dialect) WriteModels(w http.ResponseWriter, ids []string) {
	WriteModelsResponse(w, ids)
}

func (d *Dialect) ParseRequest(body []byte) (core.ParsedRequest, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return core.ParsedRequest{}, fmt.Errorf("openai: parse request: %w", err)
	}

	var pr core.ParsedRequest

	// Extract model
	if m, ok := raw["model"]; ok {
		var model string
		if err := json.Unmarshal(m, &model); err != nil {
			return core.ParsedRequest{}, fmt.Errorf("openai: parse model: %w", err)
		}
		pr.Model = model
	}

	// Extract stream
	if s, ok := raw["stream"]; ok {
		var stream bool
		if err := json.Unmarshal(s, &stream); err == nil {
			pr.Stream = stream
		}
	}

	// Extract session fingerprint inputs from messages
	if msgs, ok := raw["messages"]; ok {
		var messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(msgs, &messages); err == nil {
			// System message prefix
			for _, m := range messages {
				if m.Role == "system" {
					prefix := m.Content
					if len(prefix) > 200 {
						prefix = prefix[:200]
					}
					pr.SessionInputs = append(pr.SessionInputs, prefix)
					break
				}
			}
			// First user message
			for _, m := range messages {
				if m.Role == "user" {
					first := m.Content
					if len(first) > 500 {
						first = first[:500]
					}
					pr.SessionInputs = append(pr.SessionInputs, first)
					break
				}
			}
		}
	}

	return pr, nil
}

func (d *Dialect) RewriteModel(body []byte, model string) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("openai: rewrite model: %w", err)
	}

	modelJSON, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	raw["model"] = modelJSON

	return json.Marshal(raw)
}

func (d *Dialect) AuthHeaders(cred credential.TokenResult, headers map[string]*string) http.Header {
	h := http.Header{}
	// Defaults
	if cred.Value != "" {
		h.Set("Authorization", "Bearer "+cred.Value)
	}
	h.Set("Content-Type", "application/json")

	// Apply overrides
	for key, val := range headers {
		if val == nil {
			h.Del(key)
		} else {
			h.Set(key, *val)
		}
	}

	return h
}

func (d *Dialect) NewUsageScanner() core.UsageScanner {
	return &usageScanner{}
}

func (d *Dialect) WriteError(w http.ResponseWriter, status int, errType string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    errType,
			"code":    nil,
		},
	}
	json.NewEncoder(w).Encode(resp)
}

func (d *Dialect) InjectUsageOption(body []byte) ([]byte, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return body, false
	}

	// Check if stream is true
	if s, ok := raw["stream"]; ok {
		var stream bool
		if err := json.Unmarshal(s, &stream); err != nil || !stream {
			return body, false
		}
	} else {
		return body, false
	}

	// Check if stream_options already present
	if _, ok := raw["stream_options"]; ok {
		return body, false // client already set it, leave untouched
	}

	// Inject stream_options.include_usage
	raw["stream_options"] = json.RawMessage(`{"include_usage":true}`)

	newBody, err := json.Marshal(raw)
	if err != nil {
		return body, false
	}
	return newBody, true
}

// usageScanner extracts usage from OpenAI SSE data chunks.
// Last chunk carrying usage wins - works for both final-chunk-only
// and every-chunk providers without branching.
type usageScanner struct {
	usage *core.Usage
}

func (s *usageScanner) Observe(data []byte) {
	// Skip [DONE] marker
	if len(data) >= 6 && string(data[:6]) == "[DONE]" {
		return
	}

	var chunk struct {
		Usage *struct {
			PromptTokens        int64 `json:"prompt_tokens"`
			CompletionTokens    int64 `json:"completion_tokens"`
			PromptTokensDetails *struct {
				CachedTokens int64 `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &chunk); err != nil {
		return
	}

	if chunk.Usage != nil {
		var cacheRead int64
		if chunk.Usage.PromptTokensDetails != nil {
			cacheRead = chunk.Usage.PromptTokensDetails.CachedTokens
		}
		s.usage = &core.Usage{
			InputTokens:     chunk.Usage.PromptTokens,
			OutputTokens:    chunk.Usage.CompletionTokens,
			CacheReadTokens: cacheRead,
			// OpenAI does not report a cache-creation equivalent, so CacheCreationTokens remains 0.
		}
	}
}

func (s *usageScanner) Usage() *core.Usage {
	return s.usage
}
