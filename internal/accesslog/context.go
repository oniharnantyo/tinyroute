package accesslog

import "context"

type contextKey struct{}

// WithRequestID returns a new Context that carries the given request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// RequestID extracts the request ID from the Context, or returns empty string if absent.
func RequestID(ctx context.Context) string {
	if id, ok := ctx.Value(contextKey{}).(string); ok {
		return id
	}
	return ""
}
