package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/history"
)

// Ensure Store implements history.Aggregator.
var _ history.Aggregator = (*Store)(nil)

// Stats returns aggregated metrics (total requests, success requests, input/output tokens, average latency)
// for records whose timestamp falls between from and to (inclusive).
// Success predicate is outcome IN ('ok', 'success').
func (s *Store) Stats(ctx context.Context, from, to time.Time) (history.WindowStats, error) {
	fromMs := from.UnixMilli()
	toMs := to.UnixMilli()

	query := `
SELECT
	COUNT(*),
	COALESCE(SUM(CASE WHEN outcome IN ('ok', 'success') THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(input_tokens), 0),
	COALESCE(SUM(output_tokens), 0),
	COALESCE(CAST(ROUND(AVG(latency_ms)) AS INTEGER), 0)
FROM requests
WHERE timestamp >= ? AND timestamp <= ?;
`
	var ws history.WindowStats
	row := s.db.QueryRowContext(ctx, query, fromMs, toMs)
	err := row.Scan(&ws.TotalRequests, &ws.SuccessRequests, &ws.InputTokens, &ws.OutputTokens, &ws.AvgLatencyMs)
	if err != nil {
		return history.WindowStats{}, fmt.Errorf("aggregate stats: %w", err)
	}
	return ws, nil
}

// StatsByProvider returns aggregate request and success counts grouped by provider for the given window.
func (s *Store) StatsByProvider(ctx context.Context, from, to time.Time) ([]history.ProviderStats, error) {
	fromMs := from.UnixMilli()
	toMs := to.UnixMilli()

	query := `
SELECT
	COALESCE(provider, ''),
	COUNT(*),
	COALESCE(SUM(CASE WHEN outcome IN ('ok', 'success') THEN 1 ELSE 0 END), 0)
FROM requests
WHERE timestamp >= ? AND timestamp <= ?
GROUP BY provider
ORDER BY COUNT(*) DESC, provider ASC;
`
	rows, err := s.db.QueryContext(ctx, query, fromMs, toMs)
	if err != nil {
		return nil, fmt.Errorf("aggregate stats by provider: %w", err)
	}
	defer rows.Close()

	var result []history.ProviderStats
	for rows.Next() {
		var ps history.ProviderStats
		if err := rows.Scan(&ps.Provider, &ps.TotalRequests, &ps.SuccessRequests); err != nil {
			return nil, fmt.Errorf("scan provider stats: %w", err)
		}
		result = append(result, ps)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider stats: %w", err)
	}
	if result == nil {
		result = []history.ProviderStats{}
	}
	return result, nil
}

// StatsByModel returns aggregate token counts grouped by model (model_served fallback to model_requested)
// for the given window, ranked by total tokens descending.
func (s *Store) StatsByModel(ctx context.Context, from, to time.Time) ([]history.ModelStats, error) {
	fromMs := from.UnixMilli()
	toMs := to.UnixMilli()

	query := `
SELECT
	COALESCE(NULLIF(model_served, ''), model_requested) AS model_name,
	COALESCE(SUM(input_tokens), 0) AS in_tokens,
	COALESCE(SUM(output_tokens), 0) AS out_tokens,
	COALESCE(SUM(input_tokens + output_tokens), 0) AS tot_tokens
FROM requests
WHERE timestamp >= ? AND timestamp <= ?
GROUP BY model_name
ORDER BY tot_tokens DESC, model_name ASC;
`
	rows, err := s.db.QueryContext(ctx, query, fromMs, toMs)
	if err != nil {
		return nil, fmt.Errorf("aggregate stats by model: %w", err)
	}
	defer rows.Close()

	var result []history.ModelStats
	for rows.Next() {
		var ms history.ModelStats
		if err := rows.Scan(&ms.Model, &ms.InputTokens, &ms.OutputTokens, &ms.TotalTokens); err != nil {
			return nil, fmt.Errorf("scan model stats: %w", err)
		}
		result = append(result, ms)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate model stats: %w", err)
	}
	if result == nil {
		result = []history.ModelStats{}
	}
	return result, nil
}

// RequestBuckets partitions the window into fixed-width buckets and returns contiguous request counts.
func (s *Store) RequestBuckets(ctx context.Context, from, to time.Time, bucketMs int64) ([]history.Bucket, error) {
	if bucketMs <= 0 {
		return nil, fmt.Errorf("bucketMs must be greater than 0")
	}

	fromMs := from.UnixMilli()
	toMs := to.UnixMilli()

	if toMs < fromMs {
		return []history.Bucket{}, nil
	}

	numBuckets := int((toMs - fromMs) / bucketMs)
	if numBuckets <= 0 {
		numBuckets = 1
	}

	buckets := make([]history.Bucket, numBuckets)
	for i := 0; i < numBuckets; i++ {
		buckets[i] = history.Bucket{
			Timestamp: time.UnixMilli(fromMs + int64(i)*bucketMs).UTC(),
			Count:     0,
		}
	}

	query := `
SELECT
	(timestamp - ?) / ? AS bucket_idx,
	COUNT(*)
FROM requests
WHERE timestamp >= ? AND timestamp <= ?
GROUP BY bucket_idx;
`
	rows, err := s.db.QueryContext(ctx, query, fromMs, bucketMs, fromMs, toMs)
	if err != nil {
		return nil, fmt.Errorf("aggregate request buckets: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var bucketIdx int
		var count int64
		if err := rows.Scan(&bucketIdx, &count); err != nil {
			return nil, fmt.Errorf("scan bucket row: %w", err)
		}
		if bucketIdx < 0 {
			bucketIdx = 0
		} else if bucketIdx >= numBuckets {
			bucketIdx = numBuckets - 1
		}
		buckets[bucketIdx].Count += count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bucket rows: %w", err)
	}

	return buckets, nil
}
