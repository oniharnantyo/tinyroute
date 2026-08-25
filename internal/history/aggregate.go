package history

import (
	"context"
	"time"
)

// WindowStats holds aggregate statistics for a time window.
// The success predicate is outcome IN ('ok', 'success').
type WindowStats struct {
	TotalRequests   int64
	SuccessRequests int64
	InputTokens     int64
	OutputTokens    int64
	AvgLatencyMs    int64
}

// ProviderStats holds per-provider aggregate request statistics.
type ProviderStats struct {
	Provider        string
	TotalRequests   int64
	SuccessRequests int64
}

// ModelStats holds per-model token usage statistics.
type ModelStats struct {
	Model        string
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
}

// Bucket holds request count for a fixed time bucket.
type Bucket struct {
	Timestamp time.Time
	Count     int64
}

// Aggregator provides window-scoped aggregate queries over history records.
// Success predicate is outcome IN ('ok', 'success').
type Aggregator interface {
	Stats(ctx context.Context, from, to time.Time) (WindowStats, error)
	StatsByProvider(ctx context.Context, from, to time.Time) ([]ProviderStats, error)
	StatsByModel(ctx context.Context, from, to time.Time) ([]ModelStats, error)
	RequestBuckets(ctx context.Context, from, to time.Time, bucketMs int64) ([]Bucket, error)
}
