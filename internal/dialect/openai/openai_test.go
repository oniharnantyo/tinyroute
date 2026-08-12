package openai

import (
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/credential"
)

func TestUsageScanner_CachedTokens(t *testing.T) {
	d := &Dialect{}
	s := d.NewUsageScanner()

	chunk := []byte(`{"usage":{"prompt_tokens":100,"completion_tokens":50,"prompt_tokens_details":{"cached_tokens":30}}}`)
	s.Observe(chunk)

	u := s.Usage()
	if u == nil {
		t.Fatal("Usage() = nil")
	}
	if u.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", u.InputTokens)
	}
	if u.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", u.OutputTokens)
	}
	if u.CacheReadTokens != 30 {
		t.Errorf("CacheReadTokens = %d, want 30", u.CacheReadTokens)
	}
	if u.CacheCreationTokens != 0 {
		t.Errorf("CacheCreationTokens = %d, want 0", u.CacheCreationTokens)
	}
}

func TestAuthHeaders(t *testing.T) {
	d := &Dialect{}
	h := d.AuthHeaders(credential.TokenResult{Value: "sk-test", Kind: credential.KindStatic}, nil)
	if h.Get("Authorization") != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want Bearer sk-test", h.Get("Authorization"))
	}
}

func TestPaths(t *testing.T) {
	d := &Dialect{}
	if len(d.Paths()) != 1 || d.Paths()[0] != "/v1/chat/completions" {
		t.Errorf("Paths() = %v, want [/v1/chat/completions]", d.Paths())
	}
	if len(d.MountPaths()) != 1 || d.MountPaths()[0] != "/openai/v1/chat/completions" {
		t.Errorf("MountPaths() = %v, want [/openai/v1/chat/completions]", d.MountPaths())
	}
}
