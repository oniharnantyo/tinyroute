package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/history"
	"github.com/oniharnantyo/tinyroute/internal/history/sqlite"
)

func TestOpen_LifecycleAndMigration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test_history.db")

	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Re-open (idempotent)
	db2, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Re-Open failed: %v", err)
	}
	defer db2.Close()
}

func TestSchemaMigration_FromLegacyColumns(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy_history.db")

	// Create legacy database with old column names (ts, model_req, req_body, resp_body, etc.)
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	legacySchema := `
CREATE TABLE requests (
  id                    TEXT PRIMARY KEY,
  ts                    INTEGER NOT NULL,
  provider              TEXT,
  model_req             TEXT NOT NULL,
  model_served          TEXT,
  key_id                TEXT,
  session               TEXT,
  endpoint              TEXT,
  stream                INTEGER NOT NULL,
  outcome               TEXT NOT NULL,
  input_tokens          INTEGER NOT NULL DEFAULT 0,
  output_tokens         INTEGER NOT NULL DEFAULT 0,
  cache_read_tokens     INTEGER NOT NULL DEFAULT 0,
  cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
  latency_ms            INTEGER,
  req_body              TEXT,
  resp_body             TEXT,
  xlated_req_body       TEXT,
  raw_resp_body         TEXT,
  attempts              TEXT NOT NULL DEFAULT '[]'
);
`
	if _, err := sqlDB.Exec(legacySchema); err != nil {
		t.Fatalf("failed to create legacy schema: %v", err)
	}
	sqlDB.Close()

	// Re-open via sqlite.Open which triggers Migrate() and renames old columns to new names
	migratedDB, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Migrate failed on legacy DB: %v", err)
	}
	defer migratedDB.Close()

	store := sqlite.NewStore(migratedDB)
	rec := core.RequestRecord{
		ID:                    "legacy-req-1",
		Timestamp:             time.Now().UTC(),
		ModelReq:              "claude-3-5-sonnet",
		Outcome:               core.OutcomeOK,
		RequestBody:           `{"messages":[]}`,
		ResponseBody:          `{"id":"msg-1"}`,
		TranslatedRequestBody: `{"messages":[]}`,
		RawResponseBody:       `{"id":"msg-1"}`,
	}
	store.Record(context.Background(), rec)

	rows, _, err := store.List(context.Background(), history.Filter{})
	if err != nil {
		t.Fatalf("List error after migration: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].ID != "legacy-req-1" || rows[0].RequestBody != "" {
		t.Errorf("unexpected row data after migration: %+v", rows[0])
	}

	// Verify Get returns full bodies after migration
	recGet, found, err := store.Get(context.Background(), "legacy-req-1")
	if err != nil {
		t.Fatalf("Get error after migration: %v", err)
	}
	if !found {
		t.Fatalf("expected legacy-req-1 to be found")
	}
	if recGet.RequestBody != `{"messages":[]}` || recGet.ResponseBody != `{"id":"msg-1"}` {
		t.Errorf("unexpected body after migration: req=%q, resp=%q", recGet.RequestBody, recGet.ResponseBody)
	}
}

func TestRecord_And_Query(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test_history.db")

	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	ctx := context.Background()

	now := time.Now().Truncate(time.Millisecond).UTC()

	rec1 := core.RequestRecord{
		Version:               1,
		Timestamp:             now.Add(-2 * time.Hour),
		ID:                    "req-1",
		Key:                   "key-a",
		Session:               "sess-1",
		Endpoint:              "/anthropic/v1/messages",
		ModelReq:              "claude-3-5-sonnet",
		Stream:                true,
		Outcome:               core.OutcomeOK,
		Provider:              "anthropic",
		Latency:               250 * time.Millisecond,
		RequestBody:           `{"model":"claude-3-5-sonnet","messages":[]}`,
		ResponseBody:          `{"id":"msg-1","type":"message"}`,
		TranslatedRequestBody: `{"model":"claude-3-5-sonnet","messages":[],"translated":true}`,
		RawResponseBody:       `{"id":"msg-1","type":"message","raw":true}`,
		Attempts: []core.Attempt{
			{Provider: "anthropic", Model: "claude-3-5-sonnet", Status: 200, Elapsed: 250 * time.Millisecond},
		},
		Usage: &core.Usage{
			InputTokens:         100,
			OutputTokens:        50,
			CacheReadTokens:     20,
			CacheCreationTokens: 10,
		},
	}

	rec2 := core.RequestRecord{
		Version:   1,
		Timestamp: now.Add(-1 * time.Hour),
		ID:        "req-2",
		Key:       "key-b",
		Session:   "sess-2",
		Endpoint:  "/openai/v1/chat/completions",
		ModelReq:  "gpt-4o",
		Stream:    false,
		Outcome:   core.OutcomeOK,
		Provider:  "openai",
		Latency:   180 * time.Millisecond,
		Attempts: []core.Attempt{
			{Provider: "openai", Model: "gpt-4o", Status: 200, Elapsed: 180 * time.Millisecond},
		},
		Usage: &core.Usage{
			InputTokens:     80,
			OutputTokens:    40,
			CacheReadTokens: 10,
		},
	}

	recFailed := core.RequestRecord{
		Version:   1,
		Timestamp: now,
		ID:        "req-3",
		Key:       "key-a",
		Endpoint:  "/anthropic/v1/messages",
		ModelReq:  "claude-3-5-sonnet",
		Outcome:   core.OutcomeChainExhausted,
		Provider:  "", // empty for failed
		Latency:   500 * time.Millisecond,
		Attempts: []core.Attempt{
			{Provider: "anthropic", Model: "claude-3-5-sonnet", Status: 500, Elapsed: 500 * time.Millisecond},
		},
		Usage: nil, // nil usage
	}

	store.Record(ctx, rec1)
	store.Record(ctx, rec2)
	store.Record(ctx, recFailed)

	// Query all (default limit)
	rows, nextCursor, err := store.List(ctx, history.Filter{})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if nextCursor != "" {
		t.Errorf("expected empty nextCursor, got %q", nextCursor)
	}

	// Order should be most-recent-first (req-3, req-2, req-1)
	if rows[0].ID != "req-3" || rows[1].ID != "req-2" || rows[2].ID != "req-1" {
		t.Errorf("unexpected order: %s, %s, %s", rows[0].ID, rows[1].ID, rows[2].ID)
	}

	// Verify rec1 detailed fields from List (bodies should be empty)
	r1 := rows[2]
	if r1.Provider != "anthropic" || r1.InputTokens != 100 || r1.OutputTokens != 50 || r1.CacheReadTokens != 20 || r1.CacheCreationTokens != 10 {
		t.Errorf("rec1 usage fields mismatch: %+v", r1)
	}
	if r1.Latency != 250*time.Millisecond {
		t.Errorf("rec1 latency mismatch: %v", r1.Latency)
	}
	if r1.RequestBody != "" || r1.ResponseBody != "" || r1.TranslatedRequestBody != "" || r1.RawResponseBody != "" {
		t.Errorf("rec1 in List should not have body fields: %+v", r1)
	}

	// Verify Get returns full body fields
	g1, found, err := store.Get(ctx, "req-1")
	if err != nil {
		t.Fatalf("Get req-1 error: %v", err)
	}
	if !found {
		t.Fatalf("expected req-1 to be found")
	}
	if g1.RequestBody != `{"model":"claude-3-5-sonnet","messages":[]}` || g1.ResponseBody != `{"id":"msg-1","type":"message"}` {
		t.Errorf("g1 request_body/response_body mismatch: req=%q, resp=%q", g1.RequestBody, g1.ResponseBody)
	}
	if g1.TranslatedRequestBody != `{"model":"claude-3-5-sonnet","messages":[],"translated":true}` || g1.RawResponseBody != `{"id":"msg-1","type":"message","raw":true}` {
		t.Errorf("g1 translated_request_body/raw_response_body mismatch: xlated=%q, raw_resp=%q", g1.TranslatedRequestBody, g1.RawResponseBody)
	}

	// Verify failed record nil usage behavior (defaults to 0)
	r3 := rows[0]
	if r3.Provider != "" || r3.InputTokens != 0 || r3.OutputTokens != 0 || r3.CacheReadTokens != 0 || r3.CacheCreationTokens != 0 {
		t.Errorf("recFailed usage fields mismatch: %+v", r3)
	}
}

func TestGet_ExistingAndUnknown(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	ctx := context.Background()

	// Unknown ID returns (zero, false, nil)
	summary, found, err := store.Get(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("unexpected error for nonexistent ID: %v", err)
	}
	if found {
		t.Errorf("expected found=false for nonexistent ID, got found=true")
	}
	if summary.ID != "" {
		t.Errorf("expected zero summary for nonexistent ID, got %+v", summary)
	}

	// Existing record
	now := time.Now().Truncate(time.Millisecond).UTC()
	store.Record(ctx, core.RequestRecord{
		ID:                    "req-detail-1",
		Timestamp:             now,
		Provider:              "openai",
		ModelReq:              "gpt-4o",
		Outcome:               core.OutcomeOK,
		RequestBody:           `{"prompt":"hello"}`,
		ResponseBody:          `{"choice":"world"}`,
		TranslatedRequestBody: `{"model":"gpt-4o","prompt":"hello"}`,
		RawResponseBody:       `{"raw":"data"}`,
		Attempts: []core.Attempt{
			{Provider: "openai", Model: "gpt-4o", Status: 200, Elapsed: 120 * time.Millisecond},
		},
	})

	got, found, err := store.Get(ctx, "req-detail-1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if !found {
		t.Fatalf("expected req-detail-1 to be found")
	}
	if got.ID != "req-detail-1" || got.Provider != "openai" || got.RequestBody != `{"prompt":"hello"}` {
		t.Errorf("mismatched summary: %+v", got)
	}
	if got.Attempts == "" {
		t.Errorf("expected non-empty attempts string")
	}
}

func TestList_LimitClamp(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	ctx := context.Background()

	// Insert 505 records to meaningfully verify the 500 limit clamp
	now := time.Now()
	for i := 1; i <= 505; i++ {
		store.Record(ctx, core.RequestRecord{
			ID:        fmt.Sprintf("req-clamp-%04d", i),
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			ModelReq:  "gpt-4o",
			Outcome:   core.OutcomeOK,
		})
	}

	// Query with Limit = 1000 (exceeding MaxListLimit of 500)
	rows, nextCursor, err := store.List(ctx, history.Filter{Limit: 1000})
	if err != nil {
		t.Fatalf("List error with large limit: %v", err)
	}
	if len(rows) != history.MaxListLimit {
		t.Errorf("expected %d rows clamped, got %d", history.MaxListLimit, len(rows))
	}
	if nextCursor == "" {
		t.Errorf("expected nextCursor to be non-empty when records exceed limit")
	}
}

func TestDuplicateID_Policy(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	ctx := context.Background()

	rec := core.RequestRecord{
		ID:        "dup-id",
		Timestamp: time.Now(),
		ModelReq:  "original-model",
		Outcome:   core.OutcomeOK,
	}
	store.Record(ctx, rec)

	dupRec := core.RequestRecord{
		ID:        "dup-id",
		Timestamp: time.Now(),
		ModelReq:  "overwritten-model",
		Outcome:   core.OutcomeOK,
	}
	store.Record(ctx, dupRec)

	rows, _, err := store.List(ctx, history.Filter{})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].ModelReq != "original-model" {
		t.Errorf("expected original row to be preserved under INSERT OR IGNORE, got %s", rows[0].ModelReq)
	}
}

func TestQuery_FilteringAndPagination(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	ctx := context.Background()

	baseTime := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	// Insert 5 records
	for i := 1; i <= 5; i++ {
		provider := "anthropic"
		if i%2 == 0 {
			provider = "openai"
		}
		store.Record(ctx, core.RequestRecord{
			ID:        fmt.Sprintf("req-%d", i),
			Timestamp: baseTime.Add(time.Duration(i) * time.Hour),
			Provider:  provider,
			ModelReq:  "test-model",
			Outcome:   core.OutcomeOK,
		})
	}

	// Filter by provider
	rows, _, err := store.List(ctx, history.Filter{Provider: "anthropic"})
	if err != nil {
		t.Fatalf("List provider filter error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 anthropic rows, got %d", len(rows))
	}

	// Filter by date range (From hour 2 to hour 4)
	rows, _, err = store.List(ctx, history.Filter{
		From: baseTime.Add(2 * time.Hour),
		To:   baseTime.Add(4 * time.Hour),
	})
	if err != nil {
		t.Fatalf("List date filter error: %v", err)
	}
	if len(rows) != 3 { // req-4, req-3, req-2
		t.Fatalf("expected 3 rows in date range, got %d", len(rows))
	}

	// Pagination with Limit=2
	page1, cursor1, err := store.List(ctx, history.Filter{Limit: 2})
	if err != nil {
		t.Fatalf("Page 1 error: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("Page 1 expected 2 rows, got %d", len(page1))
	}
	if page1[0].ID != "req-5" || page1[1].ID != "req-4" {
		t.Errorf("Page 1 unexpected IDs: %s, %s", page1[0].ID, page1[1].ID)
	}
	if cursor1 == "" {
		t.Fatal("Page 1 expected non-empty nextCursor")
	}

	// Page 2
	page2, cursor2, err := store.List(ctx, history.Filter{Limit: 2, Cursor: cursor1})
	if err != nil {
		t.Fatalf("Page 2 error: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("Page 2 expected 2 rows, got %d", len(page2))
	}
	if page2[0].ID != "req-3" || page2[1].ID != "req-2" {
		t.Errorf("Page 2 unexpected IDs: %s, %s", page2[0].ID, page2[1].ID)
	}
	if cursor2 == "" {
		t.Fatal("Page 2 expected non-empty nextCursor")
	}

	// Page 3
	page3, cursor3, err := store.List(ctx, history.Filter{Limit: 2, Cursor: cursor2})
	if err != nil {
		t.Fatalf("Page 3 error: %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("Page 3 expected 1 row, got %d", len(page3))
	}
	if page3[0].ID != "req-1" {
		t.Errorf("Page 3 unexpected ID: %s", page3[0].ID)
	}
	if cursor3 != "" {
		t.Errorf("Page 3 expected empty cursor, got %q", cursor3)
	}

	// No-match returns empty slice, no error
	noMatch, _, err := store.List(ctx, history.Filter{Provider: "nonexistent"})
	if err != nil {
		t.Fatalf("No-match error: %v", err)
	}
	if len(noMatch) != 0 {
		t.Errorf("expected 0 rows for non-matching provider, got %d", len(noMatch))
	}
}

func TestQuery_KeyIDAndSessionFiltering(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	ctx := context.Background()

	baseTime := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	store.Record(ctx, core.RequestRecord{
		ID:        "req-1",
		Timestamp: baseTime.Add(1 * time.Hour),
		Key:       "key-alpha",
		Session:   "sess-1",
		Provider:  "anthropic",
		ModelReq:  "claude-3-5-sonnet",
		Outcome:   core.OutcomeOK,
	})
	store.Record(ctx, core.RequestRecord{
		ID:        "req-2",
		Timestamp: baseTime.Add(2 * time.Hour),
		Key:       "key-alpha",
		Session:   "sess-2",
		Provider:  "openai",
		ModelReq:  "gpt-4o",
		Outcome:   core.OutcomeOK,
	})
	store.Record(ctx, core.RequestRecord{
		ID:        "req-3",
		Timestamp: baseTime.Add(3 * time.Hour),
		Key:       "key-beta",
		Session:   "sess-1",
		Provider:  "anthropic",
		ModelReq:  "claude-3-5-sonnet",
		Outcome:   core.OutcomeOK,
	})

	// Filter by KeyID
	rows, _, err := store.List(ctx, history.Filter{KeyID: "key-alpha"})
	if err != nil {
		t.Fatalf("List KeyID filter error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows for key-alpha, got %d", len(rows))
	}
	if rows[0].ID != "req-2" || rows[1].ID != "req-1" {
		t.Errorf("unexpected rows for key-alpha: %s, %s", rows[0].ID, rows[1].ID)
	}

	// Filter by Session
	rows, _, err = store.List(ctx, history.Filter{Session: "sess-1"})
	if err != nil {
		t.Fatalf("List Session filter error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows for sess-1, got %d", len(rows))
	}
	if rows[0].ID != "req-3" || rows[1].ID != "req-1" {
		t.Errorf("unexpected rows for sess-1: %s, %s", rows[0].ID, rows[1].ID)
	}

	// Combined filter (KeyID + Session)
	rows, _, err = store.List(ctx, history.Filter{KeyID: "key-alpha", Session: "sess-1"})
	if err != nil {
		t.Fatalf("List KeyID+Session filter error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row for key-alpha + sess-1, got %d", len(rows))
	}
	if rows[0].ID != "req-1" {
		t.Errorf("unexpected row for key-alpha + sess-1: %s", rows[0].ID)
	}
}

func TestLastUseByKey(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	ctx := context.Background()

	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 8, 1, 14, 0, 0, 0, time.UTC)

	// Insert records for key-1 (t1, t3), key-2 (t2), and empty key
	store.Record(ctx, core.RequestRecord{
		ID:        "req-1",
		Timestamp: t1,
		Key:       "key-1",
		Outcome:   core.OutcomeOK,
	})
	store.Record(ctx, core.RequestRecord{
		ID:        "req-2",
		Timestamp: t2,
		Key:       "key-2",
		Outcome:   core.OutcomeOK,
	})
	store.Record(ctx, core.RequestRecord{
		ID:        "req-3",
		Timestamp: t3,
		Key:       "key-1",
		Outcome:   core.OutcomeOK,
	})
	store.Record(ctx, core.RequestRecord{
		ID:        "req-4",
		Timestamp: t3,
		Key:       "",
		Outcome:   core.OutcomeOK,
	})

	lastUseMap, err := store.LastUseByKey(ctx)
	if err != nil {
		t.Fatalf("LastUseByKey error: %v", err)
	}

	if len(lastUseMap) != 2 {
		t.Fatalf("expected 2 keys in lastUseMap, got %d", len(lastUseMap))
	}

	if !lastUseMap["key-1"].Equal(t3) {
		t.Errorf("expected key-1 last use to be %v, got %v", t3, lastUseMap["key-1"])
	}
	if !lastUseMap["key-2"].Equal(t2) {
		t.Errorf("expected key-2 last use to be %v, got %v", t2, lastUseMap["key-2"])
	}
	if _, ok := lastUseMap[""]; ok {
		t.Errorf("expected empty key to be excluded from lastUseMap")
	}
}
