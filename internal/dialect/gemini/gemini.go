package gemini

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

const sessionFingerprintPrefixLen = 200

// Dialect implements core.Dialect for Google's native Gemini API.
type Dialect struct{}

func (d *Dialect) Name() string { return "gemini" }

func (d *Dialect) Paths() []string { return []string{"/v1beta/models"} }

func (d *Dialect) MountPaths() []string { return []string{"/gemini/v1beta/models"} }

// ModelsMountPath returns the inbound path for Gemini-shaped model discovery.
func (d *Dialect) ModelsMountPath() string { return "/gemini/v1/models" }

func (d *Dialect) WriteModels(w http.ResponseWriter, ids []string) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"models": ids,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (d *Dialect) ParseRequest(body []byte) (core.ParsedRequest, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return core.ParsedRequest{}, fmt.Errorf("gemini: parse request: %w", err)
	}

	var pr core.ParsedRequest

	if m, ok := raw["model"]; ok {
		_ = json.Unmarshal(m, &pr.Model)
	}

	if s, ok := raw["stream"]; ok {
		_ = json.Unmarshal(s, &pr.Stream)
	}

	// Extract session fingerprint inputs
	// 1) systemInstruction or system message
	if sys, ok := raw["systemInstruction"]; ok {
		if len(sys) > sessionFingerprintPrefixLen {
			pr.SessionInputs = append(pr.SessionInputs, sys[:sessionFingerprintPrefixLen])
		} else {
			pr.SessionInputs = append(pr.SessionInputs, sys)
		}
	} else if msgs, ok := raw["messages"]; ok {
		var messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(msgs, &messages); err == nil {
			for _, m := range messages {
				if m.Role == "system" {
					prefix := m.Content
					if len(prefix) > sessionFingerprintPrefixLen {
						prefix = prefix[:sessionFingerprintPrefixLen]
					}
					pr.SessionInputs = append(pr.SessionInputs, prefix)
					break
				}
			}
		}
	}

	// 2) contents or first user message
	if contents, ok := raw["contents"]; ok {
		var items []json.RawMessage
		if err := json.Unmarshal(contents, &items); err == nil && len(items) > 0 {
			first := items[0]
			if len(first) > 500 {
				first = first[:500]
			}
			pr.SessionInputs = append(pr.SessionInputs, first)
		}
	} else if msgs, ok := raw["messages"]; ok {
		var messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(msgs, &messages); err == nil {
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
		return nil, fmt.Errorf("gemini: rewrite model: %w", err)
	}

	modelJSON, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("gemini: encode model: %w", err)
	}
	raw["model"] = modelJSON

	out, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("gemini: encode body: %w", err)
	}
	return out, nil
}

func (d *Dialect) AuthHeaders(cred credential.TokenResult, headers map[string]*string) http.Header {
	h := http.Header{}
	if cred.Value != "" {
		if cred.Kind == credential.KindOAuthBearer {
			h.Set("Authorization", "Bearer "+cred.Value)
		} else {
			h.Set("x-goog-api-key", cred.Value)
		}
	}
	h.Set("Content-Type", "application/json")

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

	if errType == "" {
		errType = http.StatusText(status)
	}

	resp := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    status,
			"message": message,
			"status":  errType,
		},
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func (d *Dialect) InjectUsageOption(body []byte) ([]byte, bool) {
	return body, false
}

type usageScanner struct {
	usage *core.Usage
}

type geminiSSEChunk struct {
	UsageMetadata *struct {
		PromptTokenCount        int64 `json:"promptTokenCount"`
		CandidatesTokenCount    int64 `json:"candidatesTokenCount"`
		TotalTokenCount         int64 `json:"totalTokenCount"`
		CachedContentTokenCount int64 `json:"cachedContentTokenCount"`
	} `json:"usageMetadata"`
}

func (s *usageScanner) Observe(data []byte) {
	if len(data) >= 6 && string(data[:6]) == "[DONE]" {
		return
	}

	var chunk geminiSSEChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return
	}

	if chunk.UsageMetadata != nil {
		s.usage = &core.Usage{
			InputTokens:     chunk.UsageMetadata.PromptTokenCount,
			OutputTokens:    chunk.UsageMetadata.CandidatesTokenCount,
			CacheReadTokens: chunk.UsageMetadata.CachedContentTokenCount,
		}
	}
}

func (s *usageScanner) Usage() *core.Usage {
	return s.usage
}
