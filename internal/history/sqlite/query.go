package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/history"
)

// encodeCursor builds an opaque cursor string from (ts, id).
func encodeCursor(ts int64, id string) string {
	raw := fmt.Sprintf("%d:%s", ts, id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor extracts (ts, id) from an opaque cursor string.
func decodeCursor(cursor string) (int64, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, "", fmt.Errorf("invalid cursor encoding: %w", err)
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("invalid cursor format")
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid cursor timestamp: %w", err)
	}
	return ts, parts[1], nil
}

// List queries history summaries matching the filter with cursor pagination.
func (s *Store) List(ctx context.Context, filter history.Filter) ([]history.Summary, string, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	var where []string
	var args []interface{}

	if filter.Provider != "" {
		where = append(where, "provider = ?")
		args = append(args, filter.Provider)
	}

	if filter.KeyID != "" {
		where = append(where, "key_id = ?")
		args = append(args, filter.KeyID)
	}

	if filter.Session != "" {
		where = append(where, "session = ?")
		args = append(args, filter.Session)
	}

	if !filter.From.IsZero() {
		where = append(where, "timestamp >= ?")
		args = append(args, filter.From.UnixMilli())
	}

	if !filter.To.IsZero() {
		where = append(where, "timestamp <= ?")
		args = append(args, filter.To.UnixMilli())
	}

	if filter.Cursor != "" {
		curTS, curID, err := decodeCursor(filter.Cursor)
		if err != nil {
			return nil, "", fmt.Errorf("list history: %w", err)
		}
		where = append(where, "(timestamp < ? OR (timestamp = ? AND id < ?))")
		args = append(args, curTS, curTS, curID)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	// Fetch limit+1 rows to determine if there is a next page
	query := fmt.Sprintf(`
SELECT id, timestamp, provider, model_requested, model_served, key_id, session, endpoint,
       stream, outcome, input_tokens, output_tokens, cache_read_tokens,
       cache_creation_tokens, latency_ms, request_body, response_body, translated_request_body,
       raw_response_body, attempts
FROM requests
%s
ORDER BY timestamp DESC, id DESC
LIMIT %d;
`, whereClause, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("query history requests: %w", err)
	}
	defer rows.Close()

	var summaries []history.Summary
	for rows.Next() {
		var sRow history.Summary
		var tsMs int64
		var streamInt int
		var latencyMs sql.NullInt64
		var provider, modelServed, keyID, session, endpoint sql.NullString
		var reqBody, respBody, xlatedReqBody, rawRespBody, attempts sql.NullString

		err := rows.Scan(
			&sRow.ID,
			&tsMs,
			&provider,
			&sRow.ModelReq,
			&modelServed,
			&keyID,
			&session,
			&endpoint,
			&streamInt,
			&sRow.Outcome,
			&sRow.InputTokens,
			&sRow.OutputTokens,
			&sRow.CacheReadTokens,
			&sRow.CacheCreationTokens,
			&latencyMs,
			&reqBody,
			&respBody,
			&xlatedReqBody,
			&rawRespBody,
			&attempts,
		)
		if err != nil {
			return nil, "", fmt.Errorf("scan history row: %w", err)
		}

		sRow.Timestamp = time.UnixMilli(tsMs).UTC()
		sRow.Stream = streamInt != 0
		if provider.Valid {
			sRow.Provider = provider.String
		}
		if modelServed.Valid {
			sRow.ModelServed = modelServed.String
		}
		if keyID.Valid {
			sRow.KeyID = keyID.String
		}
		if session.Valid {
			sRow.Session = session.String
		}
		if endpoint.Valid {
			sRow.Endpoint = endpoint.String
		}
		if latencyMs.Valid {
			sRow.Latency = time.Duration(latencyMs.Int64) * time.Millisecond
		}
		if reqBody.Valid {
			sRow.RequestBody = reqBody.String
		}
		if respBody.Valid {
			sRow.ResponseBody = respBody.String
		}
		if xlatedReqBody.Valid {
			sRow.TranslatedRequestBody = xlatedReqBody.String
		}
		if rawRespBody.Valid {
			sRow.RawResponseBody = rawRespBody.String
		}
		if attempts.Valid {
			sRow.Attempts = attempts.String
		}

		summaries = append(summaries, sRow)
	}

	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate history rows: %w", err)
	}

	nextCursor := ""
	if len(summaries) > limit {
		last := summaries[limit-1]
		nextCursor = encodeCursor(last.Timestamp.UnixMilli(), last.ID)
		summaries = summaries[:limit]
	}

	if summaries == nil {
		summaries = []history.Summary{}
	}

	return summaries, nextCursor, nil
}

// LastUseByKey returns a map of key_id to the most recent request timestamp.
func (s *Store) LastUseByKey(ctx context.Context) (map[string]time.Time, error) {
	query := `
SELECT key_id, MAX(timestamp)
FROM requests
WHERE key_id IS NOT NULL AND key_id != ''
GROUP BY key_id;
`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query last use by key: %w", err)
	}
	defer rows.Close()

	result := make(map[string]time.Time)
	for rows.Next() {
		var keyID string
		var tsMs int64
		if err := rows.Scan(&keyID, &tsMs); err != nil {
			return nil, fmt.Errorf("scan last use row: %w", err)
		}
		result[keyID] = time.UnixMilli(tsMs).UTC()
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate last use rows: %w", err)
	}

	return result, nil
}
