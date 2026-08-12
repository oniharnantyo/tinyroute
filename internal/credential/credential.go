package credential

import (
	"context"
)

// TokenKind identifies the type of outbound credential token.
type TokenKind string

const (
	// KindStatic represents a static API key (e.g. x-api-key or Authorization header).
	KindStatic TokenKind = "static"
	// KindOAuthBearer represents an OAuth 2.0 Bearer access token.
	KindOAuthBearer TokenKind = "oauth_bearer"
)

// TokenResult holds the resolved token string and its kind tag.
type TokenResult struct {
	Value string
	Kind  TokenKind
}

// Credential represents a strategy that yields a current token on demand per request hop.
type Credential interface {
	Token(ctx context.Context) (TokenResult, error)
}

// StaticKey implements Credential for fixed, non-refreshable API keys.
type StaticKey struct {
	key string
}

// NewStaticKey constructs a StaticKey credential.
func NewStaticKey(key string) *StaticKey {
	return &StaticKey{key: key}
}

// Token returns the configured static API key immediately with no network or disk I/O.
func (s *StaticKey) Token(ctx context.Context) (TokenResult, error) {
	return TokenResult{
		Value: s.key,
		Kind:  KindStatic,
	}, nil
}
