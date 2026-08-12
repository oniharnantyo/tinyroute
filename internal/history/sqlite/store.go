package sqlite

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/oniharnantyo/tinyroute/internal/core"
)

// Store implements core.Recorder for SQLite database storage.
type Store struct {
	*DB
}

// NewStore wraps a DB as a core.Recorder implementation.
func NewStore(db *DB) *Store {
	return &Store{DB: db}
}

// Record implements core.Recorder. Called off the critical path to persist a RequestRecord.
func (s *Store) Record(ctx context.Context, rec core.RequestRecord) {
	attemptsJSON, err := json.Marshal(rec.Attempts)
	if err != nil {
		attemptsJSON = []byte("[]")
	}

	var inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int64
	if rec.Usage != nil {
		inputTokens = rec.Usage.InputTokens
		outputTokens = rec.Usage.OutputTokens
		cacheReadTokens = rec.Usage.CacheReadTokens
		cacheCreationTokens = rec.Usage.CacheCreationTokens
	}

	modelServed := ""
	for _, a := range rec.Attempts {
		if a.Status >= 200 && a.Status < 300 {
			modelServed = a.Model
			break
		}
	}

	streamInt := 0
	if rec.Stream {
		streamInt = 1
	}

	latencyMs := rec.Latency.Milliseconds()

	account := ""
	provName := rec.Provider
	if idx := strings.Index(rec.Provider, "/"); idx != -1 {
		provName = rec.Provider[:idx]
		account = rec.Provider[idx+1:]
	}

	query := `
INSERT OR IGNORE INTO requests (
	id, timestamp, provider, account, model_requested, model_served, key_id, session, endpoint,
	stream, outcome, input_tokens, output_tokens, cache_read_tokens,
	cache_creation_tokens, latency_ms, request_body, response_body, translated_request_body,
	raw_response_body, attempts
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
`

	_, err = s.db.ExecContext(
		ctx,
		query,
		rec.ID,
		rec.Timestamp.UnixMilli(),
		provName,
		account,
		rec.ModelReq,
		modelServed,
		rec.Key,
		rec.Session,
		rec.Endpoint,
		streamInt,
		string(rec.Outcome),
		inputTokens,
		outputTokens,
		cacheReadTokens,
		cacheCreationTokens,
		latencyMs,
		rec.RequestBody,
		rec.ResponseBody,
		rec.TranslatedRequestBody,
		rec.RawResponseBody,
		string(attemptsJSON),
	)
	if err != nil {
		slog.Error("sqlite record failed", "id", rec.ID, "err", err)
	}
}
