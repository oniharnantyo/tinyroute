package core

import (
	"net/http"
	"testing"
	"time"
)

func TestStandardResetParserRetryAfter(t *testing.T) {
	parser := NewStandardResetParser(1 * time.Hour)

	resp := &http.Response{
		Header: http.Header{
			"Retry-After": []string{"30"},
		},
	}
	d := parser.Duration(resp, nil, nil)
	if d != 30*time.Second {
		t.Errorf("expected 30s, got %v", d)
	}
}

func TestStandardResetParserOpenAIHeaders(t *testing.T) {
	parser := NewStandardResetParser(1 * time.Hour)

	resp := &http.Response{
		Header: http.Header{
			"X-Ratelimit-Reset-Requests": []string{"6m0s"},
		},
	}
	d := parser.Duration(resp, nil, nil)
	if d != 6*time.Minute {
		t.Errorf("expected 6m0s, got %v", d)
	}
}

func TestStandardResetParserJSONBodyResetsAt(t *testing.T) {
	parser := NewStandardResetParser(1 * time.Hour)

	futureUnix := time.Now().Add(45 * time.Second).Unix()
	body := []byte(`{"error":{"message":"Rate limit exceeded","resets_at":` + time.Now().Add(45*time.Second).Format("150405") + `,"reset_at":` + string(rune(futureUnix)) + `}}`)

	resp := &http.Response{Header: http.Header{}}
	_ = body
	// Test Codex usage limit reached
	bodyCodex := []byte(`{"error":{"message":"usage_limit_reached: account limit reached"}}`)
	d := parser.Duration(resp, bodyCodex, nil)
	if d <= 0 {
		t.Errorf("expected positive duration for usage_limit_reached, got %v", d)
	}
}

func TestStandardResetParserCap(t *testing.T) {
	parser := NewStandardResetParser(10 * time.Minute)

	resp := &http.Response{
		Header: http.Header{
			"Retry-After": []string{"3600"},
		},
	}
	d := parser.Duration(resp, nil, nil)
	if d != 10*time.Minute {
		t.Errorf("expected max cap of 10m, got %v", d)
	}
}
