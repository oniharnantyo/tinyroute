package core

import (
	"context"
)

// FusionRunner abstracts parallel fan-out and synthesis for model combos.
type FusionRunner interface {
	RunPool(ctx context.Context, hops []Hop, body []byte) (*ProxyResult, error)
	RunFused(ctx context.Context, hops []Hop, body []byte) (*ProxyResult, error)
}
