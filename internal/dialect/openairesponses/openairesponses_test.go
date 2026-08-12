package openairesponses

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/credential"
)

func TestParseRequest(t *testing.T) {
	d := &Dialect{}
	body := []byte(`{"model":"gpt-5.1","stream":true,"instructions":"system instructions","input":"user input"}`)

	pr, err := d.ParseRequest(body)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	if pr.Model != "gpt-5.1" {
		t.Errorf("Model = %q, want gpt-5.1", pr.Model)
	}
	if !pr.Stream {
		t.Errorf("Stream = false, want true")
	}
	if len(pr.SessionInputs) != 2 {
		t.Fatalf("SessionInputs len = %d, want 2", len(pr.SessionInputs))
	}
}

func TestRewriteModel(t *testing.T) {
	d := &Dialect{}
	body := []byte(`{"model":"gpt-5.1","input":"test"}`)

	out, err := d.RewriteModel(body, "gpt-5-turbo")
	if err != nil {
		t.Fatalf("RewriteModel failed: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	var m string
	json.Unmarshal(raw["model"], &m)
	if m != "gpt-5-turbo" {
		t.Errorf("model = %q, want gpt-5-turbo", m)
	}
}

func TestUsageScanner_ResponseCompletedReasoningTokens(t *testing.T) {
	d := &Dialect{}
	s := d.NewUsageScanner()

	chunk := []byte(`{"response":{"usage":{"input_tokens":120,"output_tokens":60,"input_tokens_details":{"cached_tokens":40},"output_tokens_details":{"reasoning_tokens":25}}}}`)
	s.Observe(chunk)

	u := s.Usage()
	if u == nil {
		t.Fatal("Usage() = nil")
	}

	if u.InputTokens != 120 {
		t.Errorf("InputTokens = %d, want 120", u.InputTokens)
	}
	if u.OutputTokens != 60 {
		t.Errorf("OutputTokens = %d, want 60", u.OutputTokens)
	}
	if u.CacheReadTokens != 40 {
		t.Errorf("CacheReadTokens = %d, want 40", u.CacheReadTokens)
	}
	if u.ReasoningTokens != 25 {
		t.Errorf("ReasoningTokens = %d, want 25", u.ReasoningTokens)
	}
}

func TestUsageScanner_SSERelayStream(t *testing.T) {
	d := &Dialect{}
	s := d.NewUsageScanner()

	// In SSE stream relay, proxy.go extracts the data payload from data: lines and passes it to Observe.
	s.Observe([]byte(`{"type":"response.created"}`))
	s.Observe([]byte(`{"type":"response.output_item.added"}`))
	s.Observe([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":200,"output_tokens":100,"input_tokens_details":{"cached_tokens":50},"output_tokens_details":{"reasoning_tokens":30}}}}`))

	u := s.Usage()
	if u == nil {
		t.Fatal("Usage() = nil")
	}

	if u.InputTokens != 200 {
		t.Errorf("InputTokens = %d, want 200", u.InputTokens)
	}
	if u.OutputTokens != 100 {
		t.Errorf("OutputTokens = %d, want 100", u.OutputTokens)
	}
	if u.CacheReadTokens != 50 {
		t.Errorf("CacheReadTokens = %d, want 50", u.CacheReadTokens)
	}
	if u.ReasoningTokens != 30 {
		t.Errorf("ReasoningTokens = %d, want 30", u.ReasoningTokens)
	}
}

func TestPathsAndMountPaths(t *testing.T) {
	d := &Dialect{}
	if len(d.Paths()) != 1 || d.Paths()[0] != "/v1/responses" {
		t.Errorf("Paths() = %v, want [/v1/responses]", d.Paths())
	}
	if len(d.MountPaths()) != 1 || d.MountPaths()[0] != "/openai/v1/responses" {
		t.Errorf("MountPaths() = %v, want [/openai/v1/responses]", d.MountPaths())
	}
}

func TestAuthHeaders(t *testing.T) {
	d := &Dialect{}
	h := d.AuthHeaders(credential.TokenResult{Value: "sk-test", Kind: credential.KindStatic}, nil)
	if h.Get("Authorization") != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want Bearer sk-test", h.Get("Authorization"))
	}
}

func TestWriteModels(t *testing.T) {
	d := &Dialect{}
	rec := httptest.NewRecorder()

	d.WriteModels(rec, []string{"gpt-5.1"})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
