package core

import (
	"net/http"
	"time"
)

// ParsedRequest holds the routing-relevant fields extracted from a request body.
type ParsedRequest struct {
	Model  string
	Stream bool
	// SessionInputs are the fields used to derive a session fingerprint
	// (system prompt prefix + first message) when no explicit session header is present.
	SessionInputs [][]byte
}

// Attempt records one hop's outcome in a proxy chain.
type Attempt struct {
	Provider string        `json:"provider"`
	Model    string        `json:"model"`
	Status   int           `json:"status"`
	Elapsed  time.Duration `json:"elapsed_ms"`
}

// Usage holds token counts from a completed request.
type Usage struct {
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`
	ReasoningTokens     int64 `json:"reasoning_tokens,omitempty"`
}

// Outcome categorizes the final result of a proxied request.
type Outcome string

const (
	OutcomeOK             Outcome = "ok"
	OutcomeMidStream      Outcome = "mid_stream_failure"
	OutcomeChainExhausted Outcome = "chain_exhausted"
	OutcomeNoRoute        Outcome = "no_route"
	OutcomeAuthFailed     Outcome = "auth_failed"
	OutcomeRateLimited    Outcome = "rate_limited"
	OutcomeBodyTooLarge   Outcome = "body_too_large"
)

// FailureClass determines retry and cooldown behavior for a failed attempt.
type FailureClass int

const (
	// FailureRetryWithCooldown: connection error, 5xx, 429 — retry next hop, cooldown this provider.
	FailureRetryWithCooldown FailureClass = iota
	// FailureRetryNoCooldown: 404 — retry next hop, no cooldown (model absent at this provider).
	FailureRetryNoCooldown
	// FailureNoRetryWithCooldown: 401/403 — do not retry, cooldown 15min, surface warning.
	FailureNoRetryWithCooldown
	// FailureNoRetryNoCooldown: other 4xx — do not retry, no cooldown, return as-is.
	FailureNoRetryNoCooldown
)

// ClassifyFailure determines the failure class from an HTTP status code and whether
// it was a connection-level error (status 0).
func ClassifyFailure(status int) FailureClass {
	switch {
	case status == 0: // connection error / timeout
		return FailureRetryWithCooldown
	case status == 429:
		return FailureRetryWithCooldown
	case status >= 500:
		return FailureRetryWithCooldown
	case status == 404:
		return FailureRetryNoCooldown
	case status == 401 || status == 403:
		return FailureNoRetryWithCooldown
	default: // other 4xx
		return FailureNoRetryNoCooldown
	}
}

// RequestRecord is the complete record written to history for one proxied request.
type RequestRecord struct {
	Version               int           `json:"version"`
	Timestamp             time.Time     `json:"timestamp"`
	ID                    string        `json:"id"`
	Key                   string        `json:"key"`
	Session               string        `json:"session"`
	Endpoint              string        `json:"endpoint"`
	ModelReq              string        `json:"model_requested"`
	Stream                bool          `json:"stream"`
	Attempts              []Attempt     `json:"attempts"`
	Usage                 *Usage        `json:"usage,omitempty"`
	Outcome               Outcome       `json:"outcome"`
	Latency               time.Duration `json:"latency_ms,omitempty"`
	Provider              string        `json:"provider,omitempty"`
	RequestBody           string        `json:"request_body,omitempty"`
	ResponseBody          string        `json:"response_body,omitempty"`
	TranslatedRequestBody string        `json:"translated_request_body,omitempty"`
	RawResponseBody       string        `json:"raw_response_body,omitempty"`
}

// Hop represents one step in a route chain: provider name, optional account, and model to request.
type Hop struct {
	Provider     string
	Account      string   // optional pinned account or strategy pool ("default", "acc1", etc.)
	Model        string   // "$model" means passthrough the client's requested model
	Mode         string   // optional execution mode: "ordered", "pool", "fused"
	Capabilities []string // optional required capabilities
}

// HopKey returns provider/account key for health and credential store lookups.
func (h Hop) HopKey() string {
	if h.Account != "" && h.Account != "default" {
		return h.Provider + "/" + h.Account
	}
	return h.Provider
}

// ResolvedRoute is the result of route resolution: the chain of hops to attempt.
type ResolvedRoute struct {
	Hops         []Hop
	ComboName    string
	Mode         string
	Capabilities []string
}

// ProxyResult is returned by the proxy after handling a request.
type ProxyResult struct {
	Attempts []Attempt
	Usage    *Usage
	Outcome  Outcome
	ReqBody  []byte
	RespBody []byte
}

// ErrorResponse is a generic container for returning errors to clients.
type ErrorResponse struct {
	Status  int
	Headers http.Header
	Body    []byte
}
