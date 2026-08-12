package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/history"
	"github.com/oniharnantyo/tinyroute/internal/history/sqlite"
)

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = w

	execErr := fn()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	r.Close()

	return buf.String(), execErr
}

func TestCLI_HistoryCommands(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "history.db")
	t.Setenv("TINYROUTE_HISTORY_DB", dbPath)

	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("sqlite.Open failed: %v", err)
	}

	store := sqlite.NewStore(db)
	ctx := context.Background()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	// Record sample request records
	rec1 := core.RequestRecord{
		ID:        "req-1",
		Timestamp: now.Add(-2 * time.Hour),
		Key:       "key-user1",
		Session:   "sess-alpha",
		Endpoint:  "/anthropic/v1/messages",
		ModelReq:  "claude-3-5-sonnet",
		Outcome:   core.OutcomeOK,
		Provider:  "anthropic",
		Latency:   200 * time.Millisecond,
		Attempts: []core.Attempt{
			{Provider: "anthropic", Model: "claude-3-5-sonnet", Status: 200, Elapsed: 200 * time.Millisecond},
		},
		Usage: &core.Usage{InputTokens: 50, OutputTokens: 100},
	}

	rec2 := core.RequestRecord{
		ID:        "req-2",
		Timestamp: now.Add(-1 * time.Hour),
		Key:       "key-user1",
		Session:   "sess-alpha",
		Endpoint:  "/anthropic/v1/messages",
		ModelReq:  "claude-3-5-sonnet",
		Outcome:   core.OutcomeOK,
		Provider:  "anthropic",
		Latency:   150 * time.Millisecond,
		Attempts: []core.Attempt{
			{Provider: "anthropic", Model: "claude-3-5-sonnet", Status: 200, Elapsed: 150 * time.Millisecond},
		},
		Usage: &core.Usage{InputTokens: 30, OutputTokens: 60},
	}

	rec3 := core.RequestRecord{
		ID:        "req-3",
		Timestamp: now,
		Key:       "key-user2",
		Session:   "sess-beta",
		Endpoint:  "/openai/v1/chat/completions",
		ModelReq:  "gpt-4o",
		Outcome:   core.OutcomeChainExhausted,
		Provider:  "",
		Latency:   500 * time.Millisecond,
		Attempts: []core.Attempt{
			{Provider: "openai", Model: "gpt-4o", Status: 500, Elapsed: 500 * time.Millisecond},
		},
	}

	store.Record(ctx, rec1)
	store.Record(ctx, rec2)
	store.Record(ctx, rec3)

	db.Close()

	// 1. Test cmdLog without flags & assert key in output
	t.Run("cmdLog basic output assertion", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return cmdLog(nil)
		})
		if err != nil {
			t.Fatalf("cmdLog failed: %v", err)
		}
		if !strings.Contains(out, "key=key-user1") {
			t.Errorf("cmdLog output missing key=key-user1, got:\n%s", out)
		}
		if !strings.Contains(out, "key=key-user2") {
			t.Errorf("cmdLog output missing key=key-user2, got:\n%s", out)
		}
	})

	// 2. Test cmdLog with filters
	t.Run("cmdLog filters output assertion", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return cmdLog([]string{"--failures", "--key=key-user2", "--session=sess-beta"})
		})
		if err != nil {
			t.Fatalf("cmdLog with filters failed: %v", err)
		}
		if !strings.Contains(out, "key=key-user2") {
			t.Errorf("filtered cmdLog missing key=key-user2, got:\n%s", out)
		}
		if strings.Contains(out, "key-user1") {
			t.Errorf("filtered cmdLog should not contain key-user1, got:\n%s", out)
		}
	})

	// 3. Test cmdSessions basic & assert KEY column
	t.Run("cmdSessions KEY column assertion", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return cmdSessions(nil)
		})
		if err != nil {
			t.Fatalf("cmdSessions failed: %v", err)
		}
		if !strings.Contains(out, "KEY") {
			t.Errorf("cmdSessions header missing KEY column, got:\n%s", out)
		}
		if !strings.Contains(out, "key-user1") || !strings.Contains(out, "key-user2") {
			t.Errorf("cmdSessions missing key identifiers, got:\n%s", out)
		}
	})

	// 4. Test cmdSessions with key filter
	t.Run("cmdSessions key filter", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return cmdSessions([]string{"--key=key-user1"})
		})
		if err != nil {
			t.Fatalf("cmdSessions key filter failed: %v", err)
		}
		if !strings.Contains(out, "sess-alpha") {
			t.Errorf("cmdSessions key filter missing sess-alpha, got:\n%s", out)
		}
		if strings.Contains(out, "sess-beta") {
			t.Errorf("cmdSessions key filter should exclude sess-beta, got:\n%s", out)
		}
	})

	// 5. Test cmdSessionReplay & assert key in turn header
	t.Run("cmdSessionReplay turn key assertion", func(t *testing.T) {
		out, err := captureStdout(t, func() error {
			return cmdSessionReplay("sess-alpha")
		})
		if err != nil {
			t.Fatalf("cmdSessionReplay sess-alpha failed: %v", err)
		}
		if !strings.Contains(out, "key=key-user1") {
			t.Errorf("cmdSessionReplay missing key=key-user1 in turn header, got:\n%s", out)
		}

		if err := cmdSessionReplay("nonexistent"); err == nil {
			t.Errorf("expected error for nonexistent session, got nil")
		}
	})

	// 6. Test deriveLastUse happy & error paths
	t.Run("deriveLastUse happy and error paths", func(t *testing.T) {
		lastUse := deriveLastUse(dbPath)
		if len(lastUse) != 2 {
			t.Fatalf("expected 2 keys in lastUse, got %d", len(lastUse))
		}
		if !lastUse["key-user1"].Equal(now.Add(-1 * time.Hour)) {
			t.Errorf("expected key-user1 last use %v, got %v", now.Add(-1*time.Hour), lastUse["key-user1"])
		}
		if !lastUse["key-user2"].Equal(now) {
			t.Errorf("expected key-user2 last use %v, got %v", now, lastUse["key-user2"])
		}

		// Error path (invalid path)
		emptyLastUse := deriveLastUse(filepath.Join(dir, "nonexistent_dir", "invalid.db"))
		if len(emptyLastUse) != 0 {
			t.Errorf("expected empty map on invalid db path, got %v", emptyLastUse)
		}

		// Error path (corrupted file where Query fails)
		corruptPath := filepath.Join(dir, "corrupt.db")
		_ = os.WriteFile(corruptPath, []byte("not a sqlite database file"), 0644)
		corruptLastUse := deriveLastUse(corruptPath)
		if len(corruptLastUse) != 0 {
			t.Errorf("expected empty map on corrupt db file, got %v", corruptLastUse)
		}
	})

	// 7. Test pollFollowHistory directly
	t.Run("pollFollowHistory polling loop", func(t *testing.T) {
		dbFollow, err := sqlite.Open(dbPath)
		if err != nil {
			t.Fatalf("sqlite.Open failed: %v", err)
		}
		defer dbFollow.Close()
		storeFollow := sqlite.NewStore(dbFollow)

		pollCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()

		var printed []string
		matches := func(rec history.Summary) bool { return true }
		printLine := func(rec history.Summary) { printed = append(printed, rec.ID) }

		err = pollFollowHistory(pollCtx, storeFollow, 10*time.Millisecond, now.Add(-3*time.Hour), "", time.Time{}, "", "", matches, printLine)
		if err != nil {
			t.Errorf("pollFollowHistory failed: %v", err)
		}
		if len(printed) == 0 {
			t.Errorf("expected pollFollowHistory to print records, got 0")
		}
	})
}
