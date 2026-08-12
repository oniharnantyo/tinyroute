package concerns_test

import (
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/translate/concerns"
)

func TestFromOpenAIFinish(t *testing.T) {
	tests := []struct {
		reason string
		target string
		want   string
	}{
		{"stop", "anthropic", "end_turn"},
		{"length", "anthropic", "max_tokens"},
		{"tool_calls", "anthropic", "tool_use"},
		{"function_call", "anthropic", "tool_use"},
		{"content_filter", "anthropic", "content_filter"},
		{"", "anthropic", "end_turn"},
		{"unknown_reason", "anthropic", "end_turn"},
		{"stop", "other", "stop"},
	}

	for _, tt := range tests {
		got := concerns.FromOpenAIFinish(tt.reason, tt.target)
		if got != tt.want {
			t.Errorf("FromOpenAIFinish(%q, %q) = %q, want %q", tt.reason, tt.target, got, tt.want)
		}
	}
}

func TestUsageFromOpenAI(t *testing.T) {
	u := concerns.UsageFromOpenAI(100, 50, 20, 10)
	if u.InputTokens != 70 { // 100 - 20 - 10 = 70
		t.Errorf("InputTokens = %d, want 70", u.InputTokens)
	}
	if u.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", u.OutputTokens)
	}
	if u.CacheReadInputTokens != 20 {
		t.Errorf("CacheReadInputTokens = %d, want 20", u.CacheReadInputTokens)
	}
	if u.CacheCreationInputTokens != 10 {
		t.Errorf("CacheCreationInputTokens = %d, want 10", u.CacheCreationInputTokens)
	}

	// Defensive clamp at zero
	uClamped := concerns.UsageFromOpenAI(10, 5, 20, 10)
	if uClamped.InputTokens != 0 {
		t.Errorf("InputTokens = %d, want 0 when prompt < cached+creation", uClamped.InputTokens)
	}
}

func TestGeminiNameMap(t *testing.T) {
	nm := concerns.NewGeminiNameMap()

	san1 := nm.Sanitize("get-weather-v1!")
	if san1 != "get-weather-v1_" {
		t.Errorf("Sanitize(get-weather-v1!) = %q, want get-weather-v1_", san1)
	}
	if restored := nm.Restore(san1); restored != "get-weather-v1!" {
		t.Errorf("Restore(%q) = %q, want get-weather-v1!", san1, restored)
	}

	// Name starting with digit
	san2 := nm.Sanitize("9router_action")
	if san2 != "_9router_action" {
		t.Errorf("Sanitize(9router_action) = %q, want _9router_action", san2)
	}
	if restored := nm.Restore(san2); restored != "9router_action" {
		t.Errorf("Restore(%q) = %q, want 9router_action", san2, restored)
	}
}
