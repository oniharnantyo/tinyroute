package proxy

import (
	"context"

	"github.com/oniharnantyo/tinyroute/internal/core"
)

type contextKey struct{}

// RequestCtx holds pre-computed values for a single request, set by the
// server/auth layer before the proxy handler runs.
type RequestCtx struct {
	Dialect   core.Dialect
	Route     core.ResolvedRoute
	Parsed    core.ParsedRequest
	RequestID string
	KeyID     string
	SessionID string
}

// WithRequestContext attaches a RequestCtx to the context.
func WithRequestContext(ctx context.Context, rc *RequestCtx) context.Context {
	return context.WithValue(ctx, contextKey{}, rc)
}

// getRequestContext retrieves the RequestCtx attached by WithRequestContext,
// or nil if none is present.
func getRequestContext(ctx context.Context) *RequestCtx {
	rc, _ := ctx.Value(contextKey{}).(*RequestCtx)
	return rc
}
