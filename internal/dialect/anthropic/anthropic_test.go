package anthropic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/credential"
)

func TestParseRequest_ModelStreamAndSessionInputs(t *testing.T) {
	d := &Dialect{}
	body := []byte(`{"model":"claude-sonnet-4","stream":true,"system":"you are a helpful assistant","messages":[{"role":"user","content":"hi"}],"max_tokens":100,"custom_future_field":{"nested":true}}`)

	pr, err := d.ParseRequest(body)
	if err != nil {
		t.Fatalf("ParseRequest error: %v", err)
	}
	if pr.Model != "claude-sonnet-4" {
		t.Errorf("Model = %q, want claude-sonnet-4", pr.Model)
	}
	if !pr.Stream {
		t.Errorf("Stream = false, want true")
	}
	if len(pr.SessionInputs) != 2 {
		t.Fatalf("SessionInputs len = %d, want 2", len(pr.SessionInputs))
	}
}

func TestRewriteModel_PreservesUnknownFields(t *testing.T) {
	d := &Dialect{}
	body := []byte(`{"model":"old-model","max_tokens":100,"custom_future_field":{"nested":true},"messages":[{"role":"user","content":"hi"}]}`)

	out, err := d.RewriteModel(body, "new-model")
	if err != nil {
		t.Fatalf("RewriteModel error: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	var model string
	json.Unmarshal(raw["model"], &model)
	if model != "new-model" {
		t.Errorf("model = %q, want new-model", model)
	}
	if _, ok := raw["custom_future_field"]; !ok {
		t.Errorf("custom_future_field dropped")
	}
	if string(raw["custom_future_field"]) != `{"nested":true}` {
		t.Errorf("custom_future_field = %s, want unchanged", raw["custom_future_field"])
	}
}

func TestAuthHeaders_DefaultsAndOverrides(t *testing.T) {
	d := &Dialect{}

	h := d.AuthHeaders(credential.TokenResult{Value: "sk-test", Kind: credential.KindStatic}, nil)
	if h.Get("x-api-key") != "sk-test" {
		t.Errorf("x-api-key = %q", h.Get("x-api-key"))
	}
	if h.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("anthropic-version = %q", h.Get("anthropic-version"))
	}

	// Test OAuth Bearer token
	hOAuth := d.AuthHeaders(credential.TokenResult{Value: "oauth-token-123", Kind: credential.KindOAuthBearer}, nil)
	if hOAuth.Get("Authorization") != "Bearer oauth-token-123" {
		t.Errorf("Authorization = %q, want Bearer oauth-token-123", hOAuth.Get("Authorization"))
	}
	if hOAuth.Get("x-api-key") != "" {
		t.Errorf("x-api-key = %q, want empty for OAuth token", hOAuth.Get("x-api-key"))
	}

	override := "custom-version"
	removed := (*string)(nil)
	h2 := d.AuthHeaders(credential.TokenResult{Value: "sk-test", Kind: credential.KindStatic}, map[string]*string{
		"anthropic-version": &override,
		"x-api-key":         removed,
	})
	if h2.Get("anthropic-version") != "custom-version" {
		t.Errorf("override anthropic-version = %q", h2.Get("anthropic-version"))
	}
	if h2.Get("x-api-key") != "" {
		t.Errorf("x-api-key should be removed, got %q", h2.Get("x-api-key"))
	}
}

func TestUsageScanner_LastChunkWins(t *testing.T) {
	d := &Dialect{}
	s := d.NewUsageScanner()

	s.Observe([]byte(`{"type":"message_start","message":{"usage":{"input_tokens":50,"cache_read_input_tokens":15,"cache_creation_input_tokens":5,"output_tokens":0}}}`))
	s.Observe([]byte(`{"type":"message_delta","usage":{"output_tokens":10}}`))
	s.Observe([]byte(`{"type":"message_delta","usage":{"output_tokens":25}}`))

	u := s.Usage()
	if u == nil {
		t.Fatal("Usage() = nil")
	}
	if u.InputTokens != 50 {
		t.Errorf("InputTokens = %d, want 50", u.InputTokens)
	}
	if u.OutputTokens != 25 {
		t.Errorf("OutputTokens = %d, want 25 (last chunk wins)", u.OutputTokens)
	}
	if u.CacheReadTokens != 15 {
		t.Errorf("CacheReadTokens = %d, want 15", u.CacheReadTokens)
	}
	if u.CacheCreationTokens != 5 {
		t.Errorf("CacheCreationTokens = %d, want 5", u.CacheCreationTokens)
	}
}

func TestWriteError_AnthropicEnvelope(t *testing.T) {
	d := &Dialect{}
	rec := httptest.NewRecorder()

	d.WriteError(rec, http.StatusServiceUnavailable, "overloaded_error", "all providers exhausted")

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d", rec.Code)
	}
	var body struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error body: %v", err)
	}
	if body.Type != "error" || body.Error.Type != "overloaded_error" || !strings.Contains(body.Error.Message, "exhausted") {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestInjectUsageOption_NoOp(t *testing.T) {
	d := &Dialect{}
	body := []byte(`{"model":"x"}`)
	out, injected := d.InjectUsageOption(body)
	if injected {
		t.Errorf("InjectUsageOption should never inject for anthropic")
	}
	if string(out) != string(body) {
		t.Errorf("body should be unchanged")
	}
}

func TestPathsAndMountPaths(t *testing.T) {
	d := &Dialect{}
	if len(d.Paths()) != 1 || d.Paths()[0] != "/v1/messages" {
		t.Errorf("Paths() = %v, want [/v1/messages]", d.Paths())
	}
	if len(d.MountPaths()) != 1 || d.MountPaths()[0] != "/anthropic/v1/messages" {
		t.Errorf("MountPaths() = %v, want [/anthropic/v1/messages]", d.MountPaths())
	}
}

func TestWriteModels_AnthropicShape(t *testing.T) {
	d := &Dialect{}
	rec := httptest.NewRecorder()

	d.WriteModels(rec, []string{"claude-3-5-sonnet-20241022", "claude-3-haiku-20240307"})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		Data []struct {
			Type        string `json:"type"`
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			CreatedAt   string `json:"created_at"`
		} `json:"data"`
		HasMore bool    `json:"has_more"`
		FirstID *string `json:"first_id"`
		LastID  *string `json:"last_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal models response: %v", err)
	}
	if resp.HasMore {
		t.Errorf("has_more = true, want false")
	}
	if len(resp.Data) != 2 {
		t.Fatalf("data len = %d, want 2", len(resp.Data))
	}
	if resp.Data[0].Type != "model" || resp.Data[0].ID != "claude-3-5-sonnet-20241022" || resp.Data[0].DisplayName != "claude-3-5-sonnet-20241022" || resp.Data[0].CreatedAt != "1970-01-01T00:00:00Z" {
		t.Errorf("unexpected entry 0: %+v", resp.Data[0])
	}
	if resp.FirstID == nil || *resp.FirstID != "claude-3-5-sonnet-20241022" {
		t.Errorf("first_id = %v", resp.FirstID)
	}
	if resp.LastID == nil || *resp.LastID != "claude-3-haiku-20240307" {
		t.Errorf("last_id = %v", resp.LastID)
	}
}
