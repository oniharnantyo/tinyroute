package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/dashboard/components"
)

func TestDecodeAttempts(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
		{
			name:     "malformed json",
			input:    "{not valid json}",
			expected: 0,
		},
		{
			name:     "valid json array with 2 attempts",
			input:    `[{"provider":"openai","model":"gpt-4o","status":429,"elapsed_ms":100},{"provider":"anthropic","model":"claude-3-5-sonnet","status":200,"elapsed_ms":250}]`,
			expected: 2,
		},
		{
			name:     "null json",
			input:    "null",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeAttempts(tt.input)
			if len(got) != tt.expected {
				t.Errorf("decodeAttempts(%q) len = %d; want %d", tt.input, len(got), tt.expected)
			}
		})
	}
}

func TestDeriveStatusCode(t *testing.T) {
	tests := []struct {
		name     string
		outcome  string
		attempts []core.Attempt
		expected int
	}{
		{
			name:    "first 2xx attempt wins",
			outcome: string(core.OutcomeOK),
			attempts: []core.Attempt{
				{Provider: "openai", Status: 500, Elapsed: 100 * time.Millisecond},
				{Provider: "anthropic", Status: 200, Elapsed: 200 * time.Millisecond},
				{Provider: "groq", Status: 503, Elapsed: 300 * time.Millisecond},
			},
			expected: 200,
		},
		{
			name:    "201 status attempt wins",
			outcome: string(core.OutcomeOK),
			attempts: []core.Attempt{
				{Provider: "openai", Status: 201, Elapsed: 100 * time.Millisecond},
			},
			expected: 201,
		},
		{
			name:    "no 2xx attempt -> last attempt status used",
			outcome: string(core.OutcomeChainExhausted),
			attempts: []core.Attempt{
				{Provider: "openai", Status: 429, Elapsed: 100 * time.Millisecond},
				{Provider: "anthropic", Status: 503, Elapsed: 200 * time.Millisecond},
			},
			expected: 503,
		},
		{
			name:    "no 2xx attempt with 0 status -> last attempt 0 returned",
			outcome: string(core.OutcomeChainExhausted),
			attempts: []core.Attempt{
				{Provider: "openai", Status: 0, Elapsed: 100 * time.Millisecond},
			},
			expected: 0,
		},
		// Outcome map fallbacks (no attempts)
		{
			name:     "no attempts, outcome no_route -> 404",
			outcome:  string(core.OutcomeNoRoute),
			attempts: nil,
			expected: 404,
		},
		{
			name:     "no attempts, outcome auth_failed -> 401",
			outcome:  string(core.OutcomeAuthFailed),
			attempts: nil,
			expected: 401,
		},
		{
			name:     "no attempts, outcome rate_limited -> 429",
			outcome:  string(core.OutcomeRateLimited),
			attempts: nil,
			expected: 429,
		},
		{
			name:     "no attempts, outcome body_too_large -> 413",
			outcome:  string(core.OutcomeBodyTooLarge),
			attempts: nil,
			expected: 413,
		},
		{
			name:     "no attempts, outcome chain_exhausted -> 502",
			outcome:  string(core.OutcomeChainExhausted),
			attempts: nil,
			expected: 502,
		},
		{
			name:     "no attempts, outcome mid_stream_failure -> 502",
			outcome:  string(core.OutcomeMidStream),
			attempts: nil,
			expected: 502,
		},
		{
			name:     "no attempts, outcome ok -> 200",
			outcome:  string(core.OutcomeOK),
			attempts: nil,
			expected: 200,
		},
		{
			name:     "no attempts, legacy outcome success -> 200",
			outcome:  "success",
			attempts: nil,
			expected: 200,
		},
		{
			name:     "no attempts, unknown outcome -> 500",
			outcome:  "unknown_error_type",
			attempts: nil,
			expected: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveStatusCode(tt.outcome, tt.attempts)
			if got != tt.expected {
				t.Errorf("deriveStatusCode(%q, %v) = %d; want %d", tt.outcome, tt.attempts, got, tt.expected)
			}
		})
	}
}

func TestHistoryStatusBadgeVariant(t *testing.T) {
	tests := []struct {
		code     int
		expected components.StatusBadgeVariant
	}{
		{code: 200, expected: components.StatusSuccess},
		{code: 201, expected: components.StatusSuccess},
		{code: 204, expected: components.StatusSuccess},
		{code: 400, expected: components.StatusWarning},
		{code: 401, expected: components.StatusWarning},
		{code: 404, expected: components.StatusWarning},
		{code: 429, expected: components.StatusWarning},
		{code: 500, expected: components.StatusError},
		{code: 502, expected: components.StatusError},
		{code: 503, expected: components.StatusError},
		{code: 0, expected: components.StatusError},
	}

	for _, tt := range tests {
		got := historyStatusBadgeVariant(tt.code)
		if got != tt.expected {
			t.Errorf("historyStatusBadgeVariant(%d) = %v; want %v", tt.code, got, tt.expected)
		}
	}
}

func TestFormatBodyPane(t *testing.T) {
	// Empty body
	empty := formatBodyPane("")
	if empty.ByteCount != 0 || empty.Content != "" || empty.IsTruncated {
		t.Errorf("unexpected empty body pane: %+v", empty)
	}

	// Valid JSON pretty print
	rawJSON := `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	formatted := formatBodyPane(rawJSON)
	if formatted.ByteCount != len(rawJSON) || !formatted.IsJSON || formatted.IsTruncated {
		t.Errorf("unexpected formatted json: %+v", formatted)
	}
	expectedSubstring := "  \"model\": \"gpt-4o\""
	if !strings.Contains(formatted.Content, expectedSubstring) {
		t.Errorf("expected indented json containing %q, got: %s", expectedSubstring, formatted.Content)
	}

	// Non-JSON plain text
	plain := "plain text error response: upstream timeout"
	formattedPlain := formatBodyPane(plain)
	if formattedPlain.ByteCount != len(plain) || formattedPlain.IsJSON || formattedPlain.Content != plain {
		t.Errorf("unexpected formatted plain text: %+v", formattedPlain)
	}

	// Large body exceeding 512 KB
	largeLen := maxBodyDisplayBytes + 1024
	largeBody := strings.Repeat("A", largeLen)
	formattedLarge := formatBodyPane(largeBody)
	if !formattedLarge.IsTruncated {
		t.Errorf("expected isTruncated=true for body exceeding 512 KB")
	}
	if formattedLarge.ByteCount != largeLen {
		t.Errorf("expected byte count %d, got %d", largeLen, formattedLarge.ByteCount)
	}
	if len(formattedLarge.Content) != maxBodyDisplayBytes {
		t.Errorf("expected truncated content length %d, got %d", maxBodyDisplayBytes, len(formattedLarge.Content))
	}
	if formattedLarge.TotalSize == "" {
		t.Errorf("expected non-empty TotalSize for truncated body")
	}

	// Oversized JSON (> 512 KB) retains IsJSON=true and is formatted then truncated
	oversizedJSON := `{"key":"` + strings.Repeat("x", maxBodyDisplayBytes+100) + `"}`
	formattedOversizedJSON := formatBodyPane(oversizedJSON)
	if !formattedOversizedJSON.IsJSON {
		t.Errorf("expected IsJSON=true for oversized JSON")
	}
	if !formattedOversizedJSON.IsTruncated {
		t.Errorf("expected IsTruncated=true for oversized JSON")
	}
	if len(formattedOversizedJSON.Content) != maxBodyDisplayBytes {
		t.Errorf("expected truncated content len %d, got %d", maxBodyDisplayBytes, len(formattedOversizedJSON.Content))
	}
	if !strings.HasPrefix(formattedOversizedJSON.Content, "{\n  \"key\": \"") {
		t.Errorf("expected formatted JSON prefix, got: %s", formattedOversizedJSON.Content[:30])
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
	}

	for _, tt := range tests {
		got := formatBytes(tt.bytes)
		if got != tt.expected {
			t.Errorf("formatBytes(%d) = %q; want %q", tt.bytes, got, tt.expected)
		}
	}
}

func TestFormatCompact(t *testing.T) {
	tests := []struct {
		n        int64
		expected string
	}{
		{0, "0"},
		{50, "50"},
		{999, "999"},
		{1000, "1.0k"},
		{1200, "1.2k"},
		{2400000, "2.4M"},
		{1500000000, "1.5B"},
		{3000000000000, "3.0T"},
		{-1200, "-1.2k"},
	}

	for _, tt := range tests {
		got := formatCompact(tt.n)
		if got != tt.expected {
			t.Errorf("formatCompact(%d) = %q; want %q", tt.n, got, tt.expected)
		}
	}
}
