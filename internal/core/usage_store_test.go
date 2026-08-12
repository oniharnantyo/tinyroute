package core

import (
	"testing"
	"time"
)

type testFakeClock struct {
	t time.Time
}

func (c *testFakeClock) Now() time.Time          { return c.t }
func (c *testFakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func TestUsageStoreRollingWindowAndExhaustion(t *testing.T) {
	fc := &testFakeClock{t: time.Now()}
	store := NewUsageStore(fc)

	quota := &QuotaConfig{
		Window:   1 * time.Hour,
		Tokens:   1000,
		Requests: 2,
	}

	key := "openai/primary"

	if store.Exhausted(key, quota) {
		t.Error("expected fresh account not to be exhausted")
	}

	// Record request 1
	store.Record(key, Usage{InputTokens: 400, OutputTokens: 200})
	if store.Exhausted(key, quota) {
		t.Error("expected account with 600 tokens not to be exhausted (limit 1000)")
	}

	// Record request 2
	store.Record(key, Usage{InputTokens: 300, OutputTokens: 200})
	// Total tokens = 1100 > 1000
	if !store.Exhausted(key, quota) {
		t.Error("expected account with 1100 tokens to be exhausted")
	}

	// Advance clock past window
	fc.Advance(61 * time.Minute)
	if store.Exhausted(key, quota) {
		t.Error("expected account after window rollover to be available")
	}
}

func TestNilUsageStoreSafety(t *testing.T) {
	var store *UsageStore
	quota := &QuotaConfig{Tokens: 100}
	if store.Exhausted("openai/acc", quota) {
		t.Error("nil usage store should never report exhausted")
	}
	store.Record("openai/acc", Usage{InputTokens: 500})
}
