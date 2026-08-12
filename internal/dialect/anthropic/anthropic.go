// Package anthropic implements core.Dialect for the Anthropic Messages API.
package anthropic

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

// sessionFingerprintPrefixLen bounds how much of the system prompt is used
// as a session fingerprint input (D9: system_prompt_prefix + messages[0]).
const sessionFingerprintPrefixLen = 200

// Dialect implements core.Dialect for the Anthropic Messages API.
type Dialect struct{}

// Name returns the dialect identifier.
func (d *Dialect) Name() string { return "anthropic" }

// Paths returns the inbound HTTP paths this dialect owns.
func (d *Dialect) Paths() []string { return []string{"/v1/messages"} }

// MountPaths returns the inbound HTTP paths this dialect mounts inbound.
func (d *Dialect) MountPaths() []string { return []string{"/anthropic/v1/messages"} }

// ModelsMountPath returns the inbound path for Anthropic-shaped model discovery.
func (d *Dialect) ModelsMountPath() string { return "/anthropic/v1/models" }

type anthropicModelEntry struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

type anthropicModelsResponse struct {
	Data    []anthropicModelEntry `json:"data"`
	HasMore bool                  `json:"has_more"`
	FirstID *string               `json:"first_id"`
	LastID  *string               `json:"last_id"`
}

func (d *Dialect) WriteModels(w http.ResponseWriter, ids []string) {
	data := make([]anthropicModelEntry, 0, len(ids))
	for _, id := range ids {
		data = append(data, anthropicModelEntry{
			Type:        "model",
			ID:          id,
			DisplayName: id,
			CreatedAt:   "1970-01-01T00:00:00Z",
		})
	}
	resp := anthropicModelsResponse{
		Data:    data,
		HasMore: false,
	}
	if len(ids) > 0 {
		first := ids[0]
		last := ids[len(ids)-1]
		resp.FirstID = &first
		resp.LastID = &last
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// ParseRequest extracts model, stream, and session fingerprint inputs
// (system prompt prefix + first message) without interpreting anything else.
func (d *Dialect) ParseRequest(body []byte) (core.ParsedRequest, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return core.ParsedRequest{}, fmt.Errorf("anthropic: parse request: %w", err)
	}

	var pr core.ParsedRequest

	if m, ok := raw["model"]; ok {
		if err := json.Unmarshal(m, &pr.Model); err != nil {
			return core.ParsedRequest{}, fmt.Errorf("anthropic: parse model: %w", err)
		}
	}

	if s, ok := raw["stream"]; ok {
		// Best-effort: an absent or malformed stream field defaults to false.
		_ = json.Unmarshal(s, &pr.Stream)
	}

	// Session fingerprint input: system prompt prefix (first N bytes of the raw value).
	if sys, ok := raw["system"]; ok {
		pr.SessionInputs = append(pr.SessionInputs, truncate(sys, sessionFingerprintPrefixLen))
	}

	// Session fingerprint input: first message content, raw.
	if msgs, ok := raw["messages"]; ok {
		var messages []json.RawMessage
		if err := json.Unmarshal(msgs, &messages); err == nil && len(messages) > 0 {
			pr.SessionInputs = append(pr.SessionInputs, messages[0])
		}
	}

	return pr, nil
}

// truncate returns the first n bytes of b, or all of b if shorter.
func truncate(b json.RawMessage, n int) []byte {
	if len(b) > n {
		return append([]byte(nil), b[:n]...)
	}
	return append([]byte(nil), b...)
}

// RewriteModel replaces the model field, preserving all other fields
// byte-for-byte via map[string]json.RawMessage round-trip.
func (d *Dialect) RewriteModel(body []byte, model string) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("anthropic: rewrite model: decode: %w", err)
	}

	modelJSON, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("anthropic: rewrite model: encode model: %w", err)
	}
	raw["model"] = modelJSON

	out, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("anthropic: rewrite model: encode: %w", err)
	}
	return out, nil
}

// AuthHeaders returns the outbound headers for authenticating with an
// Anthropic-dialect provider: x-api-key for static, Authorization: Bearer for OAuth.
// A provider's configured headers map overrides or removes (via nil) any default.
func (d *Dialect) AuthHeaders(cred credential.TokenResult, headers map[string]*string) http.Header {
	h := http.Header{}
	if cred.Value != "" {
		if cred.Kind == credential.KindOAuthBearer {
			h.Set("Authorization", "Bearer "+cred.Value)
		} else {
			h.Set("x-api-key", cred.Value)
		}
	}
	h.Set("anthropic-version", "2023-06-01")
	h.Set("content-type", "application/json")

	for key, val := range headers {
		if val == nil {
			h.Del(key)
		} else {
			h.Set(key, *val)
		}
	}

	return h
}

// NewUsageScanner returns a stateful scanner that extracts usage from
// message_start (input tokens) and message_delta (output tokens) SSE events.
func (d *Dialect) NewUsageScanner() core.UsageScanner {
	return &usageScanner{}
}

// WriteError writes the Anthropic error envelope:
// {"type":"error","error":{"type":"...","message":"..."}}
func (d *Dialect) WriteError(w http.ResponseWriter, status int, errType string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	body := struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}{Type: "error"}
	body.Error.Type = errType
	body.Error.Message = message

	_ = json.NewEncoder(w).Encode(body)
}

// InjectUsageOption is a no-op for Anthropic: usage is always reported via
// message_delta events without any client-supplied opt-in. Only relevant
// for the OpenAI dialect.
func (d *Dialect) InjectUsageOption(body []byte) ([]byte, bool) {
	return body, false
}

// usageScanner extracts token usage from Anthropic Messages SSE events.
// Contract: last chunk carrying usage wins (core.UsageScanner).
type usageScanner struct {
	usage *core.Usage
}

// anthropicUsageEvent covers the fields tinyroute cares about across
// message_start and message_delta events; unrecognized event types are
// ignored.
type anthropicUsageEvent struct {
	Type    string `json:"type"`
	Message struct {
		Usage *anthropicUsage `json:"usage"`
	} `json:"message"`
	Usage *anthropicUsage `json:"usage"`
}

type anthropicUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

// Observe processes one SSE data line.
func (s *usageScanner) Observe(data []byte) {
	var ev anthropicUsageEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return
	}

	switch ev.Type {
	case "message_start":
		if ev.Message.Usage == nil {
			return
		}
		if s.usage == nil {
			s.usage = &core.Usage{}
		}
		s.usage.InputTokens = ev.Message.Usage.InputTokens
		s.usage.CacheReadTokens = ev.Message.Usage.CacheReadInputTokens
		s.usage.CacheCreationTokens = ev.Message.Usage.CacheCreationInputTokens
		// message_start may also carry a zero output_tokens placeholder;
		// only take it if we don't yet have a value.
		if s.usage.OutputTokens == 0 {
			s.usage.OutputTokens = ev.Message.Usage.OutputTokens
		}
	case "message_delta":
		if ev.Usage == nil {
			return
		}
		if s.usage == nil {
			s.usage = &core.Usage{}
		}
		s.usage.OutputTokens = ev.Usage.OutputTokens
		if ev.Usage.InputTokens != 0 {
			s.usage.InputTokens = ev.Usage.InputTokens
		}
		if ev.Usage.CacheReadInputTokens != 0 {
			s.usage.CacheReadTokens = ev.Usage.CacheReadInputTokens
		}
		if ev.Usage.CacheCreationInputTokens != 0 {
			s.usage.CacheCreationTokens = ev.Usage.CacheCreationInputTokens
		}
	}
}

// Usage returns the accumulated usage, or nil if none was observed.
func (s *usageScanner) Usage() *core.Usage {
	return s.usage
}
