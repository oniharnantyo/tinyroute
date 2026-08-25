package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/history/sqlite"
)

func TestStats_WindowTotals_And_EmptyWindow(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	ctx := context.Background()

	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	from := now.Add(-1 * time.Hour)
	to := now

	// 1. Empty window test -> returns zeros without error
	stats, err := store.Stats(ctx, from, to)
	if err != nil {
		t.Fatalf("Stats empty window error: %v", err)
	}
	if stats.TotalRequests != 0 || stats.SuccessRequests != 0 || stats.InputTokens != 0 || stats.OutputTokens != 0 || stats.AvgLatencyMs != 0 {
		t.Errorf("expected all zeros for empty window, got: %+v", stats)
	}

	// 2. Seed records: 2 inside window, 1 before window, 1 after window
	// Before window:
	store.Record(ctx, core.RequestRecord{
		ID:        "req-before",
		Timestamp: from.Add(-10 * time.Minute),
		Provider:  "anthropic",
		ModelReq:  "claude-3-5-sonnet",
		Outcome:   core.OutcomeOK,
		Latency:   100 * time.Millisecond,
		Usage:     &core.Usage{InputTokens: 50, OutputTokens: 25},
	})

	// Inside window - record 1 (ok)
	store.Record(ctx, core.RequestRecord{
		ID:        "req-in-1",
		Timestamp: from.Add(10 * time.Minute),
		Provider:  "anthropic",
		ModelReq:  "claude-3-5-sonnet",
		Outcome:   core.OutcomeOK,
		Latency:   200 * time.Millisecond,
		Usage:     &core.Usage{InputTokens: 100, OutputTokens: 50},
	})

	// Inside window - record 2 (failed)
	store.Record(ctx, core.RequestRecord{
		ID:        "req-in-2",
		Timestamp: from.Add(20 * time.Minute),
		Provider:  "openai",
		ModelReq:  "gpt-4o",
		Outcome:   core.OutcomeRateLimited,
		Latency:   400 * time.Millisecond,
		Usage:     &core.Usage{InputTokens: 20, OutputTokens: 10},
	})

	// After window:
	store.Record(ctx, core.RequestRecord{
		ID:        "req-after",
		Timestamp: to.Add(10 * time.Minute),
		Provider:  "openai",
		ModelReq:  "gpt-4o",
		Outcome:   core.OutcomeOK,
		Latency:   150 * time.Millisecond,
		Usage:     &core.Usage{InputTokens: 70, OutputTokens: 35},
	})

	stats, err = store.Stats(ctx, from, to)
	if err != nil {
		t.Fatalf("Stats error: %v", err)
	}

	if stats.TotalRequests != 2 {
		t.Errorf("expected 2 total requests in window, got %d", stats.TotalRequests)
	}
	if stats.SuccessRequests != 1 {
		t.Errorf("expected 1 success request in window, got %d", stats.SuccessRequests)
	}
	if stats.InputTokens != 120 {
		t.Errorf("expected 120 input tokens, got %d", stats.InputTokens)
	}
	if stats.OutputTokens != 60 {
		t.Errorf("expected 60 output tokens, got %d", stats.OutputTokens)
	}
	// Avg latency of (200 + 400)/2 = 300 ms
	if stats.AvgLatencyMs != 300 {
		t.Errorf("expected 300 avg latency ms, got %d", stats.AvgLatencyMs)
	}
}

func TestStats_LegacySuccessOutcome_And_NullLatency(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	ctx := context.Background()

	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	from := now.Add(-1 * time.Hour)
	to := now

	// Record with legacy "success" outcome
	store.Record(ctx, core.RequestRecord{
		ID:        "req-legacy-success",
		Timestamp: from.Add(5 * time.Minute),
		Provider:  "anthropic",
		ModelReq:  "claude-3-5-sonnet",
		Outcome:   core.Outcome("success"),
		Latency:   100 * time.Millisecond,
		Usage:     &core.Usage{InputTokens: 40, OutputTokens: 20},
	})

	// Record with core.OutcomeOK
	store.Record(ctx, core.RequestRecord{
		ID:        "req-ok",
		Timestamp: from.Add(15 * time.Minute),
		Provider:  "anthropic",
		ModelReq:  "claude-3-5-sonnet",
		Outcome:   core.OutcomeOK,
		Latency:   200 * time.Millisecond,
		Usage:     &core.Usage{InputTokens: 60, OutputTokens: 30},
	})

	// Directly insert a record with NULL latency_ms via raw SQL to ensure NULL latency is ignored in AVG
	err = db.Close()
	if err != nil {
		t.Fatalf("Close error: %v", err)
	}
	rawSQLDB, err := sql.Open("sqlite", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("sql.Open error: %v", err)
	}
	_, err = rawSQLDB.Exec(`
INSERT INTO requests (id, timestamp, model_requested, outcome, stream, latency_ms)
VALUES ('req-null-lat', ?, 'claude-3-5-sonnet', 'ok', 0, NULL);
`, from.Add(25*time.Minute).UnixMilli())
	if err != nil {
		t.Fatalf("insert null latency row: %v", err)
	}
	rawSQLDB.Close()

	db, err = sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Re-Open error: %v", err)
	}
	defer db.Close()
	store = sqlite.NewStore(db)

	stats, err := store.Stats(ctx, from, to)
	if err != nil {
		t.Fatalf("Stats error: %v", err)
	}

	if stats.TotalRequests != 3 {
		t.Errorf("expected 3 total requests, got %d", stats.TotalRequests)
	}
	// Both "success" and "ok" (including the null latency row) should be counted as success
	if stats.SuccessRequests != 3 {
		t.Errorf("expected 3 success requests, got %d", stats.SuccessRequests)
	}
	if stats.AvgLatencyMs != 150 {
		t.Errorf("expected 150 avg latency ms ( (100+200)/2 ), got %d", stats.AvgLatencyMs)
	}
}

func TestStatsByProvider(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	ctx := context.Background()

	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	from := now.Add(-1 * time.Hour)
	to := now

	// 2 anthropic requests (1 ok, 1 fail)
	store.Record(ctx, core.RequestRecord{
		ID:        "req-a1",
		Timestamp: from.Add(5 * time.Minute),
		Provider:  "anthropic",
		Outcome:   core.OutcomeOK,
	})
	store.Record(ctx, core.RequestRecord{
		ID:        "req-a2",
		Timestamp: from.Add(10 * time.Minute),
		Provider:  "anthropic",
		Outcome:   core.OutcomeRateLimited,
	})

	// 3 openai requests (2 ok, 1 legacy success)
	store.Record(ctx, core.RequestRecord{
		ID:        "req-o1",
		Timestamp: from.Add(15 * time.Minute),
		Provider:  "openai",
		Outcome:   core.OutcomeOK,
	})
	store.Record(ctx, core.RequestRecord{
		ID:        "req-o2",
		Timestamp: from.Add(20 * time.Minute),
		Provider:  "openai",
		Outcome:   core.OutcomeOK,
	})
	store.Record(ctx, core.RequestRecord{
		ID:        "req-o3",
		Timestamp: from.Add(25 * time.Minute),
		Provider:  "openai",
		Outcome:   core.Outcome("success"),
	})

	// 1 outside window
	store.Record(ctx, core.RequestRecord{
		ID:        "req-outside",
		Timestamp: from.Add(-5 * time.Minute),
		Provider:  "anthropic",
		Outcome:   core.OutcomeOK,
	})

	pStats, err := store.StatsByProvider(ctx, from, to)
	if err != nil {
		t.Fatalf("StatsByProvider error: %v", err)
	}

	if len(pStats) != 2 {
		t.Fatalf("expected 2 provider stats, got %d", len(pStats))
	}

	// openai has 3 requests (most), anthropic has 2
	if pStats[0].Provider != "openai" || pStats[0].TotalRequests != 3 || pStats[0].SuccessRequests != 3 {
		t.Errorf("unexpected openai stats: %+v", pStats[0])
	}
	if pStats[1].Provider != "anthropic" || pStats[1].TotalRequests != 2 || pStats[1].SuccessRequests != 1 {
		t.Errorf("unexpected anthropic stats: %+v", pStats[1])
	}
}

func TestStatsByModel_ModelServedFallback(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	ctx := context.Background()

	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	from := now.Add(-1 * time.Hour)
	to := now

	// 1. Success attempt: model_requested="claude-3-5", model_served="claude-3-5-sonnet-20241022"
	store.Record(ctx, core.RequestRecord{
		ID:        "req-m1",
		Timestamp: from.Add(5 * time.Minute),
		ModelReq:  "claude-3-5",
		Attempts: []core.Attempt{
			{Provider: "anthropic", Model: "claude-3-5-sonnet-20241022", Status: 200},
		},
		Outcome: core.OutcomeOK,
		Usage:   &core.Usage{InputTokens: 100, OutputTokens: 50},
	})

	// 2. Failed attempt: model_requested="gpt-4o", no 2xx attempt -> model_served empty, fallback to model_requested
	store.Record(ctx, core.RequestRecord{
		ID:        "req-m2",
		Timestamp: from.Add(10 * time.Minute),
		ModelReq:  "gpt-4o",
		Attempts: []core.Attempt{
			{Provider: "openai", Model: "gpt-4o", Status: 500},
		},
		Outcome: core.OutcomeChainExhausted,
		Usage:   &core.Usage{InputTokens: 30, OutputTokens: 10},
	})

	// 3. Another record for claude-3-5-sonnet-20241022 with exact match
	store.Record(ctx, core.RequestRecord{
		ID:        "req-m3",
		Timestamp: from.Add(15 * time.Minute),
		ModelReq:  "claude-3-5-sonnet-20241022",
		Attempts: []core.Attempt{
			{Provider: "anthropic", Model: "claude-3-5-sonnet-20241022", Status: 200},
		},
		Outcome: core.OutcomeOK,
		Usage:   &core.Usage{InputTokens: 200, OutputTokens: 100},
	})

	mStats, err := store.StatsByModel(ctx, from, to)
	if err != nil {
		t.Fatalf("StatsByModel error: %v", err)
	}

	if len(mStats) != 2 {
		t.Fatalf("expected 2 model stats, got %d", len(mStats))
	}

	// First should be claude-3-5-sonnet-20241022 with 300 in + 150 out = 450 total tokens
	if mStats[0].Model != "claude-3-5-sonnet-20241022" || mStats[0].InputTokens != 300 || mStats[0].OutputTokens != 150 || mStats[0].TotalTokens != 450 {
		t.Errorf("unexpected mStats[0]: %+v", mStats[0])
	}

	// Second should be gpt-4o (fallback from requested) with 30 in + 10 out = 40 total tokens
	if mStats[1].Model != "gpt-4o" || mStats[1].InputTokens != 30 || mStats[1].OutputTokens != 10 || mStats[1].TotalTokens != 40 {
		t.Errorf("unexpected mStats[1]: %+v", mStats[1])
	}
}

func TestRequestBuckets_PartitionAndEmptyFill(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	ctx := context.Background()

	// 1 hour window: 12 buckets of 5 minutes (300,000 ms)
	from := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	to := from.Add(1 * time.Hour)
	bucketMs := int64(5 * 60 * 1000) // 5m

	// Record in bucket 0 (at from + 1m)
	store.Record(ctx, core.RequestRecord{
		ID:        "req-b0",
		Timestamp: from.Add(1 * time.Minute),
		Outcome:   core.OutcomeOK,
	})

	// 2 Records in bucket 2 (at from + 11m, 12m)
	store.Record(ctx, core.RequestRecord{
		ID:        "req-b2-1",
		Timestamp: from.Add(11 * time.Minute),
		Outcome:   core.OutcomeOK,
	})
	store.Record(ctx, core.RequestRecord{
		ID:        "req-b2-2",
		Timestamp: from.Add(12 * time.Minute),
		Outcome:   core.OutcomeOK,
	})

	// Record right at boundary 'to' (from + 60m)
	store.Record(ctx, core.RequestRecord{
		ID:        "req-b-last",
		Timestamp: to,
		Outcome:   core.OutcomeOK,
	})

	buckets, err := store.RequestBuckets(ctx, from, to, bucketMs)
	if err != nil {
		t.Fatalf("RequestBuckets error: %v", err)
	}

	if len(buckets) != 12 {
		t.Fatalf("expected 12 contiguous buckets, got %d", len(buckets))
	}

	// Bucket 0 should have 1
	if buckets[0].Count != 1 {
		t.Errorf("bucket 0 count expected 1, got %d", buckets[0].Count)
	}
	// Bucket 1 should be empty (0)
	if buckets[1].Count != 0 {
		t.Errorf("bucket 1 count expected 0, got %d", buckets[1].Count)
	}
	// Bucket 2 should have 2
	if buckets[2].Count != 2 {
		t.Errorf("bucket 2 count expected 2, got %d", buckets[2].Count)
	}
	// Bucket 11 (last bucket) should have 1 (the record at 'to')
	if buckets[11].Count != 1 {
		t.Errorf("bucket 11 count expected 1, got %d", buckets[11].Count)
	}

	// Verify timestamps are contiguous
	for i, b := range buckets {
		expectedTime := from.Add(time.Duration(i*5) * time.Minute)
		if !b.Timestamp.Equal(expectedTime) {
			t.Errorf("bucket %d timestamp expected %v, got %v", i, expectedTime, b.Timestamp)
		}
	}
}
