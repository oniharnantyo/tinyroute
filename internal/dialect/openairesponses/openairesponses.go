package openairesponses

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/credential"
	"github.com/oniharnantyo/tinyroute/internal/dialect"
	"github.com/oniharnantyo/tinyroute/internal/dialect/openai"
)

func init() {
	dialect.Register(&Dialect{})
}

// Dialect implements core.Dialect for the OpenAI Responses API.
type Dialect struct{}

func (d *Dialect) Name() string { return "openai-responses" }

func (d *Dialect) Paths() []string { return []string{"/v1/responses"} }

func (d *Dialect) MountPaths() []string { return []string{"/openai/v1/responses"} }

// ModelsMountPath returns "" because the Responses dialect reuses the openai
// surface's mount prefix and is served by /openai/v1/models rather than
// registering its own (orphaned) discovery endpoint.
func (d *Dialect) ModelsMountPath() string { return "" }

func (d *Dialect) ParseRequest(body []byte) (core.ParsedRequest, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return core.ParsedRequest{}, fmt.Errorf("openairesponses: parse request: %w", err)
	}

	var pr core.ParsedRequest

	if m, ok := raw["model"]; ok {
		var model string
		if err := json.Unmarshal(m, &model); err == nil {
			pr.Model = model
		}
	}

	if s, ok := raw["stream"]; ok {
		var stream bool
		if err := json.Unmarshal(s, &stream); err == nil {
			pr.Stream = stream
		}
	}

	if inst, ok := raw["instructions"]; ok {
		if len(inst) > 200 {
			pr.SessionInputs = append(pr.SessionInputs, inst[:200])
		} else {
			pr.SessionInputs = append(pr.SessionInputs, inst)
		}
	}

	if inp, ok := raw["input"]; ok {
		if len(inp) > 500 {
			pr.SessionInputs = append(pr.SessionInputs, inp[:500])
		} else {
			pr.SessionInputs = append(pr.SessionInputs, inp)
		}
	}

	return pr, nil
}

func (d *Dialect) RewriteModel(body []byte, model string) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("openairesponses: rewrite model: %w", err)
	}

	modelJSON, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("openairesponses: rewrite model: encode model: %w", err)
	}
	raw["model"] = modelJSON

	out, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("openairesponses: rewrite model: encode: %w", err)
	}
	return out, nil
}

func (d *Dialect) AuthHeaders(cred credential.TokenResult, headers map[string]*string) http.Header {
	h := http.Header{}
	if cred.Value != "" {
		h.Set("Authorization", "Bearer "+cred.Value)
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
	resp := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    errType,
			"code":    nil,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (d *Dialect) InjectUsageOption(body []byte) ([]byte, bool) {
	return body, false
}

func (d *Dialect) WriteModels(w http.ResponseWriter, ids []string) {
	openai.WriteModelsResponse(w, ids)
}

type usageScanner struct {
	usage *core.Usage
}

func (s *usageScanner) Observe(data []byte) {
	if len(data) >= 6 && string(data[:6]) == "[DONE]" {
		return
	}

	var chunk struct {
		Response *struct {
			Usage *struct {
				InputTokens        int64 `json:"input_tokens"`
				OutputTokens       int64 `json:"output_tokens"`
				InputTokensDetails *struct {
					CachedTokens int64 `json:"cached_tokens"`
				} `json:"input_tokens_details"`
				OutputTokensDetails *struct {
					ReasoningTokens int64 `json:"reasoning_tokens"`
				} `json:"output_tokens_details"`
			} `json:"usage"`
		} `json:"response"`
		Usage *struct {
			InputTokens        int64 `json:"input_tokens"`
			OutputTokens       int64 `json:"output_tokens"`
			InputTokensDetails *struct {
				CachedTokens int64 `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			OutputTokensDetails *struct {
				ReasoningTokens int64 `json:"reasoning_tokens"`
			} `json:"output_tokens_details"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(data, &chunk); err != nil {
		return
	}

	u := chunk.Usage
	if chunk.Response != nil && chunk.Response.Usage != nil {
		u = chunk.Response.Usage
	}

	if u != nil {
		var cacheRead, reasoning int64
		if u.InputTokensDetails != nil {
			cacheRead = u.InputTokensDetails.CachedTokens
		}
		if u.OutputTokensDetails != nil {
			reasoning = u.OutputTokensDetails.ReasoningTokens
		}
		s.usage = &core.Usage{
			InputTokens:     u.InputTokens,
			OutputTokens:    u.OutputTokens,
			CacheReadTokens: cacheRead,
			ReasoningTokens: reasoning,
		}
	}
}

func (s *usageScanner) Usage() *core.Usage {
	return s.usage
}
