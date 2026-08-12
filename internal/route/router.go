package route

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/core"
)

// Config holds route definitions. Obtained from config.Topology.
type Config struct {
	Routes []RouteEntry
}

// RouteEntry is a parsed route ready for matching.
type RouteEntry struct {
	From  string     // surface dialect name
	Match string     // glob pattern against model name
	Chain []core.Hop // parsed hops
}

// ParseRoutes converts raw route config strings into RouteEntries.
func ParseRoutes(routes []RawRoute) ([]RouteEntry, error) {
	entries := make([]RouteEntry, 0, len(routes))
	for i, r := range routes {
		hops, err := parseChain(r.Chain)
		if err != nil {
			return nil, fmt.Errorf("route[%d]: %w", i, err)
		}
		entries = append(entries, RouteEntry{
			From:  r.From,
			Match: r.Match,
			Chain: hops,
		})
	}
	return entries, nil
}

// RawRoute mirrors the JSON route structure for parsing.
type RawRoute struct {
	From  string
	Match string
	Chain []string
}

func parseChain(chain []string) ([]core.Hop, error) {
	hops := make([]core.Hop, 0, len(chain))
	for _, entry := range chain {
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed chain hop %q (expected provider:model)", entry)
		}
		hops = append(hops, core.Hop{
			Provider: parts[0],
			Model:    parts[1],
		})
	}
	return hops, nil
}

// Option configures a Router.
type Option func(*Router)

// WithCombos configures the router with logical model combos.
func WithCombos(combos []config.Combo) Option {
	return func(r *Router) {
		r.combos = make(map[string]config.Combo)
		for _, cb := range combos {
			r.combos[cb.Name] = cb
		}
	}
}

// WithTranslatable configures the predicate used by Resolve to determine if a
// cross-dialect hop is permitted via translation.
func WithTranslatable(predicate func(from, to string) bool) Option {
	return func(r *Router) {
		r.translatable = predicate
	}
}

// Router resolves requests to chains based on surface and model.
type Router struct {
	routes       []RouteEntry
	providers    map[string]config.Provider
	combos       map[string]config.Combo
	translatable func(from, to string) bool
}

// New creates a Router from parsed route entries and topology providers.
func New(routes []RouteEntry, providers map[string]config.Provider, opts ...Option) *Router {
	if providers == nil {
		providers = make(map[string]config.Provider)
	}
	r := &Router{
		routes:    routes,
		providers: providers,
		combos:    make(map[string]config.Combo),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *Router) canTranslate(from, to string) bool {
	if r.translatable == nil {
		return false
	}
	return r.translatable(from, to)
}

// Resolve finds the matching route, provider@account:model, or combo resolution.
func (r *Router) Resolve(surface string, model string) (core.ResolvedRoute, error) {
	// 0. Direct combo resolution by combo name
	if cb, isCombo := r.combos[model]; isCombo {
		hops, err := r.expandCombo(surface, cb, make(map[string]bool))
		if err != nil {
			return core.ResolvedRoute{}, fmt.Errorf("combo %q resolution: %w", cb.Name, err)
		}
		if len(cb.Capabilities) > 0 {
			hops = reorderCapabilities(hops, cb.Capabilities)
		}
		return core.ResolvedRoute{
			Hops:         hops,
			ComboName:    cb.Name,
			Mode:         cb.Mode,
			Capabilities: cb.Capabilities,
		}, nil
	}

	// 1. Direct provider[@account]:model prefix resolution
	if strings.Contains(model, ":") {
		parts := strings.SplitN(model, ":", 2)
		provSpec := parts[0]
		targetModel := parts[1]

		provName := provSpec
		accName := ""
		if strings.Contains(provSpec, "@") {
			sub := strings.SplitN(provSpec, "@", 2)
			provName = sub[0]
			accName = sub[1]
		}

		if cb, isCombo := r.combos[provName]; isCombo {
			hops, err := r.expandCombo(surface, cb, make(map[string]bool))
			if err != nil {
				return core.ResolvedRoute{}, fmt.Errorf("combo %q resolution: %w", cb.Name, err)
			}
			for i := range hops {
				if hops[i].Model == "$model" {
					hops[i].Model = targetModel
				}
			}
			if len(cb.Capabilities) > 0 {
				hops = reorderCapabilities(hops, cb.Capabilities)
			}
			return core.ResolvedRoute{
				Hops:         hops,
				ComboName:    cb.Name,
				Mode:         cb.Mode,
				Capabilities: cb.Capabilities,
			}, nil
		}

		prov, ok := r.providers[provName]
		if !ok {
			return core.ResolvedRoute{}, fmt.Errorf("unknown provider %q in model request %q", provName, model)
		}

		if accName != "" && accName != "default" {
			foundAcc := false
			for _, acc := range prov.Accounts {
				if acc.Name == accName {
					foundAcc = true
					break
				}
			}
			if !foundAcc {
				return core.ResolvedRoute{}, fmt.Errorf("unknown account %q for provider %q in model request %q", accName, provName, model)
			}
		}

		// Check whitelist if configured
		if len(prov.Models) > 0 {
			allowed := false
			for _, m := range prov.Models {
				if m == targetModel {
					allowed = true
					break
				}
			}
			if !allowed {
				return core.ResolvedRoute{}, fmt.Errorf("model %q is not whitelisted for provider %q", targetModel, provName)
			}
		}

		if prov.Dialect != "" && prov.Dialect != surface && !r.canTranslate(surface, prov.Dialect) {
			return core.ResolvedRoute{}, fmt.Errorf("provider %q dialect %q does not match surface %q", provName, prov.Dialect, surface)
		}

		return core.ResolvedRoute{
			Hops: []core.Hop{
				{
					Provider: provName,
					Account:  accName,
					Model:    targetModel,
				},
			},
		}, nil
	}

	// 2. Fall back to explicit routes matching surface and model glob
	for _, entry := range r.routes {
		if entry.From != surface {
			continue
		}
		matched, err := path.Match(entry.Match, model)
		if err != nil {
			continue // malformed glob, skip
		}
		if !matched {
			continue
		}
		// Found a match - resolve $model tokens
		hops := make([]core.Hop, len(entry.Chain))
		for i, hop := range entry.Chain {
			hops[i] = core.Hop{
				Provider:     hop.Provider,
				Account:      hop.Account,
				Model:        hop.Model,
				Mode:         hop.Mode,
				Capabilities: hop.Capabilities,
			}
			if hops[i].Model == "$model" {
				hops[i].Model = model
			}
			if prov, ok := r.providers[hop.Provider]; ok && prov.Dialect != "" && prov.Dialect != surface && !r.canTranslate(surface, prov.Dialect) {
				return core.ResolvedRoute{}, fmt.Errorf("provider %q dialect %q does not match surface %q", hop.Provider, prov.Dialect, surface)
			}
		}
		return core.ResolvedRoute{Hops: hops}, nil
	}

	return core.ResolvedRoute{}, fmt.Errorf("unprefixed model %q requires explicit route configuration", model)
}

func (r *Router) expandCombo(surface string, cb config.Combo, visited map[string]bool) ([]core.Hop, error) {
	if visited[cb.Name] {
		return nil, fmt.Errorf("recursive combo reference detected in %q", cb.Name)
	}
	visited[cb.Name] = true

	var hops []core.Hop
	for _, member := range cb.Members {
		if subCb, isSubCombo := r.combos[member]; isSubCombo {
			subHops, err := r.expandCombo(surface, subCb, visited)
			if err != nil {
				return nil, err
			}
			hops = append(hops, subHops...)
			continue
		}
		parts := strings.SplitN(member, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed combo member %q in combo %q", member, cb.Name)
		}
		provSpec := parts[0]
		targetModel := parts[1]

		provName := provSpec
		accName := ""
		if strings.Contains(provSpec, "@") {
			sub := strings.SplitN(provSpec, "@", 2)
			provName = sub[0]
			accName = sub[1]
		}
		prov, ok := r.providers[provName]
		if !ok {
			return nil, fmt.Errorf("combo %q references unknown provider %q", cb.Name, provName)
		}
		if prov.Dialect != "" && prov.Dialect != surface && !r.canTranslate(surface, prov.Dialect) {
			return nil, fmt.Errorf("combo %q member provider %q dialect %q does not match surface %q", cb.Name, provName, prov.Dialect, surface)
		}
		hops = append(hops, core.Hop{
			Provider:     provName,
			Account:      accName,
			Model:        targetModel,
			Mode:         cb.Mode,
			Capabilities: cb.Capabilities,
		})
	}
	return hops, nil
}

func reorderCapabilities(hops []core.Hop, caps []string) []core.Hop {
	if len(caps) == 0 {
		return hops
	}
	capWeight := map[string]int{
		"vision": 400,
		"pdf":    300,
		"audio":  200,
		"video":  100,
	}
	res := make([]core.Hop, len(hops))
	copy(res, hops)
	sort.SliceStable(res, func(i, j int) bool {
		wI := maxCapWeight(res[i].Capabilities, capWeight)
		wJ := maxCapWeight(res[j].Capabilities, capWeight)
		return wI > wJ
	})
	return res
}

func maxCapWeight(caps []string, weights map[string]int) int {
	max := 0
	for _, c := range caps {
		if w, ok := weights[strings.ToLower(c)]; ok && w > max {
			max = w
		}
	}
	return max
}

// Models returns all concrete model names from route patterns, chain hops, provider whitelists, and combos
// that resolve successfully on the given surface dialect.
func (r *Router) Models(surface string) []string {
	seen := make(map[string]bool)
	var candidates []string

	// Add combos
	for cbName := range r.combos {
		if !seen[cbName] {
			seen[cbName] = true
			candidates = append(candidates, cbName)
		}
	}

	// Add provider whitelisted models
	for provName, prov := range r.providers {
		for _, m := range prov.Models {
			prefixed := provName + ":" + m
			if !seen[prefixed] {
				seen[prefixed] = true
				candidates = append(candidates, prefixed)
			}
			if !seen[m] {
				seen[m] = true
				candidates = append(candidates, m)
			}
		}
	}

	for _, entry := range r.routes {
		if entry.From != surface {
			continue
		}
		// Add concrete (non-glob) match patterns
		if !strings.ContainsAny(entry.Match, "*?[") {
			if !seen[entry.Match] {
				seen[entry.Match] = true
				candidates = append(candidates, entry.Match)
			}
		}
		// Add concrete models from chain hops
		for _, hop := range entry.Chain {
			if hop.Model != "$model" && !seen[hop.Model] {
				seen[hop.Model] = true
				candidates = append(candidates, hop.Model)
			}
		}
	}

	var models []string
	for _, id := range candidates {
		if _, err := r.Resolve(surface, id); err == nil {
			models = append(models, id)
		}
	}

	sort.Strings(models)
	return models
}
