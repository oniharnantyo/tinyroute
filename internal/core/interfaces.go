package core

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/credential"
)

// Dialect owns everything wire-format-specific for one API protocol.
// Implementations: anthropic, openai (and future: gemini-native, openai-responses).
type Dialect interface {
	// Name returns the dialect identifier (e.g. "anthropic", "openai").
	Name() string

	// Paths returns the outbound upstream HTTP path templates for this dialect (e.g. ["/v1/chat/completions"]).
	Paths() []string

	// MountPaths returns the inbound HTTP paths this dialect mounts inbound (e.g. ["/openai/v1/chat/completions"]).
	MountPaths() []string

	// ModelsMountPath returns the inbound HTTP path for this dialect's model
	// discovery endpoint (e.g. "/openai/v1/models"), or "" if the dialect does
	// not expose one — typically because it shares another dialect's mount
	// prefix (openai-responses reuses the openai surface and is served by
	// /openai/v1/models rather than registering its own).
	ModelsMountPath() string

	// WriteModels writes a model discovery response in this dialect's native format.
	WriteModels(w http.ResponseWriter, ids []string)

	// ParseRequest extracts routing-relevant fields from the request body.
	ParseRequest(body []byte) (ParsedRequest, error)

	// RewriteModel replaces the model field in the body, preserving all other fields.
	RewriteModel(body []byte, model string) ([]byte, error)

	// AuthHeaders returns the outbound headers for authenticating with a provider.
	// cred is the resolved token result (value + token kind). headers are provider-level header overrides
	// (null value means remove the default header).
	AuthHeaders(cred credential.TokenResult, headers map[string]*string) http.Header

	// NewUsageScanner returns a stateful scanner that observes SSE chunks
	// and extracts token usage. Contract: last chunk carrying usage wins.
	NewUsageScanner() UsageScanner

	// WriteError writes an error response in this dialect's native envelope format.
	WriteError(w http.ResponseWriter, status int, errType string, message string)

	// InjectUsageOption modifies a streaming request body to request usage reporting
	// if the client hasn't already done so. Returns the (possibly modified) body and
	// whether injection occurred. Only relevant for OpenAI dialect.
	InjectUsageOption(body []byte) ([]byte, bool)
}

// UsageScanner observes SSE event data and extracts token usage.
// Contract: call Observe for each SSE data line; Usage() returns the
// most recent usage seen (last chunk carrying usage wins).
type UsageScanner interface {
	// Observe processes one SSE data line.
	Observe(data []byte)
	// Usage returns the accumulated usage, or nil if none was observed.
	Usage() *Usage
}

// RequestTranslator converts a request body between dialects.
type RequestTranslator interface {
	// TranslateRequest converts a request body from its source dialect to the
	// target dialect. State may carry per-request metadata such as tool name maps.
	// The model field is preserved; the caller rewrites it afterwards with the target
	// dialect's RewriteModel.
	TranslateRequest(body []byte, state *StreamState) ([]byte, error)
}

// ResponseTranslator converts an upstream response chunk into zero or more
// outbound frames, mutating the provided stream state. A nil chunk signals
// end-of-stream: emit any buffered closing frames. One method covers both
// mid-stream events and the end-of-stream drain — there is deliberately no
// separate Flush method.
type ResponseTranslator interface {
	// TranslateResponse converts one upstream chunk into outbound frames.
	// Returning an empty/nil slice means the chunk produced no frames (e.g. a
	// non-JSON line to skip).
	TranslateResponse(chunk []byte, state *StreamState) (frames [][]byte, err error)
}

// Recorder writes request records to persistent storage.
type Recorder interface {
	// Record writes a completed request record. Called off the critical path.
	Record(ctx context.Context, rec RequestRecord)
}

// Selector picks which hop to try next from available (non-cooled-down) hops.
// Second implementation: weighted, latency-ranked.
type Selector interface {
	// Select returns the ordered list of hops to attempt, filtered by availability.
	Select(hops []Hop, available func(provider string) bool) []Hop
}

// CredentialStore retrieves provider credentials.
// Second implementation: macOS Keychain, systemd LoadCredential.
type CredentialStore interface {
	// Get returns the credential for a provider, or empty string if not found.
	Get(provider string) string
}

// Clock abstracts time for testing cooldowns and expiry.
// Second implementation: fake (for tests).
type Clock interface {
	Now() time.Time
}

// HealthStore tracks provider cooldowns.
type HealthStore interface {
	// Available returns true if the provider is not in a cooldown window.
	Available(provider string) bool
	// AvailableModel returns true if neither the per-model key nor provider key is in a cooldown window.
	AvailableModel(provider, model string) bool
	// Penalize records a failure for a provider with the given cooldown duration.
	Penalize(provider string, duration time.Duration)
	// PenalizeModel records a failure for a provider and model with the given cooldown duration.
	PenalizeModel(provider, model string, duration time.Duration)
	// CooldownEnd returns when the cooldown expires for a provider, or zero if available.
	CooldownEnd(provider string) time.Time
	// Save persists cooldown state.
	Save() error
	// Load restores cooldown state, discarding expired entries.
	Load() error
}

// KeyVerifier authenticates inbound requests.
type KeyVerifier interface {
	// Verify checks the bearer token and returns the key identifier if valid.
	// Returns an error describing why verification failed (absent, unknown, disabled, expired, scope).
	Verify(token string, surface string, model string) (keyID string, err error)
}

// RateLimiter checks per-key rate limits.
type RateLimiter interface {
	// Allow checks if a request from the given key is within its rate limit.
	// Returns true if allowed, false with a retry-after duration if rate limited.
	Allow(keyID string) (bool, time.Duration)
}

// Router resolves a request to a chain of hops.
type Router interface {
	// Resolve finds the matching route for the given surface dialect and model.
	Resolve(surface string, model string) (ResolvedRoute, error)
}

// SSEWriter supports streaming SSE to the client with per-chunk flushing.
type SSEWriter interface {
	io.Writer
	Flush()
}
