package history

import (
	"context"
	"time"
)

// Summary represents a history record.
type Summary struct {
	ID                    string        `json:"id"`
	Timestamp             time.Time     `json:"timestamp"`
	Provider              string        `json:"provider,omitempty"`
	ModelReq              string        `json:"model_requested"`
	ModelServed           string        `json:"model_served,omitempty"`
	KeyID                 string        `json:"key_id,omitempty"`
	Session               string        `json:"session,omitempty"`
	Endpoint              string        `json:"endpoint,omitempty"`
	Stream                bool          `json:"stream"`
	Outcome               string        `json:"outcome"`
	InputTokens           int64         `json:"input_tokens"`
	OutputTokens          int64         `json:"output_tokens"`
	CacheReadTokens       int64         `json:"cache_read_tokens"`
	CacheCreationTokens   int64         `json:"cache_creation_tokens"`
	Latency               time.Duration `json:"latency_ms,omitempty"`
	RequestBody           string        `json:"request_body,omitempty"`
	ResponseBody          string        `json:"response_body,omitempty"`
	TranslatedRequestBody string        `json:"translated_request_body,omitempty"`
	RawResponseBody       string        `json:"raw_response_body,omitempty"`
	Attempts              string        `json:"attempts,omitempty"`
}

// Filter specifies criteria for querying history records.
type Filter struct {
	Provider string    // Filter by serving provider ("" for all)
	KeyID    string    // Filter by authenticating key ID ("" for all)
	Session  string    // Filter by session ID ("" for all)
	From     time.Time // Filter records on or after From (zero value for unbounded)
	To       time.Time // Filter records on or before To (zero value for unbounded)
	Limit    int       // Max records to return (defaults to 50 if <= 0)
	Cursor   string    // Opaque cursor from a previous page
}

// MaxListLimit is the maximum number of records that can be requested in a single List call.
const MaxListLimit = 500

// Querier provides read access to history summaries.
type Querier interface {
	Get(ctx context.Context, id string) (Summary, bool, error)
	List(ctx context.Context, filter Filter) (rows []Summary, nextCursor string, err error)
	LastUseByKey(ctx context.Context) (map[string]time.Time, error)
}
