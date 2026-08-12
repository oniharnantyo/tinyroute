package route

import "github.com/oniharnantyo/tinyroute/internal/core"

// OrderedSelector implements core.Selector with ordered-first selection.
// Hops are attempted in declaration order, skipping unavailable providers.
type OrderedSelector struct{}

// Select returns hops filtered to only available providers, preserving order.
func (s *OrderedSelector) Select(hops []core.Hop, available func(provider string) bool) []core.Hop {
	result := make([]core.Hop, 0, len(hops))
	for _, hop := range hops {
		if available(hop.Provider) {
			result = append(result, hop)
		}
	}
	return result
}
