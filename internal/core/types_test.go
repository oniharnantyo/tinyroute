package core_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/core"
)

func TestUsage_CacheCreationTokens(t *testing.T) {
	u := core.Usage{
		InputTokens:         100,
		OutputTokens:        50,
		CacheReadTokens:     20,
		CacheCreationTokens: 10,
	}

	if u.CacheCreationTokens == u.CacheReadTokens {
		t.Fatalf("expected CacheCreationTokens (%d) to be distinct from CacheReadTokens (%d)", u.CacheCreationTokens, u.CacheReadTokens)
	}

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("failed to marshal usage: %v", err)
	}

	var decoded core.Usage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal usage: %v", err)
	}

	if decoded.CacheCreationTokens != 10 {
		t.Errorf("expected CacheCreationTokens 10, got %d", decoded.CacheCreationTokens)
	}
	if decoded.CacheReadTokens != 20 {
		t.Errorf("expected CacheReadTokens 20, got %d", decoded.CacheReadTokens)
	}
}

func TestRequestRecord_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	rec := core.RequestRecord{
		Version:   1,
		Timestamp: now,
		ID:        "req-123",
		Key:       "key-abc",
		Session:   "sess-xyz",
		Endpoint:  "/anthropic/v1/messages",
		ModelReq:  "claude-3-5-sonnet-20241022",
		Stream:    true,
		Attempts:  []core.Attempt{{Provider: "anthropic", Model: "claude-3-5-sonnet-20241022", Status: 200, Elapsed: 150 * time.Millisecond}},
		Usage:     &core.Usage{InputTokens: 10, OutputTokens: 20, CacheReadTokens: 5, CacheCreationTokens: 15},
		Outcome:   core.OutcomeOK,
		Latency:   150 * time.Millisecond,
		Provider:  "anthropic",
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("failed to marshal record: %v", err)
	}

	var decoded core.RequestRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal record: %v", err)
	}

	if decoded.Latency != rec.Latency {
		t.Errorf("expected Latency %v, got %v", rec.Latency, decoded.Latency)
	}
	if decoded.Provider != rec.Provider {
		t.Errorf("expected Provider %q, got %q", rec.Provider, decoded.Provider)
	}
}
