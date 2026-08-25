package route

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/core"
)

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
	providers    map[string]config.Provider
	combos       map[string]config.Combo
	translatable func(from, to string) bool
}

// New creates a Router from topology providers and options.
func New(providers map[string]config.Provider, opts ...Option) *Router {
	if providers == nil {
		providers = make(map[string]config.Provider)
	}
	r := &Router{
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

func (r *Router) resolveCombo(surface string, cb config.Combo, targetModel string) (core.ResolvedRoute, error) {
	if !cb.IsEnabled() {
		return core.ResolvedRoute{}, fmt.Errorf("combo %q is disabled", cb.Name)
	}
	hops, err := r.expandCombo(surface, cb, make(map[string]bool))
	if err != nil {
		return core.ResolvedRoute{}, fmt.Errorf("combo %q resolution: %w", cb.Name, err)
	}
	if targetModel != "" {
		for i := range hops {
			if hops[i].Model == "$model" {
				hops[i].Model = targetModel
			}
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

// Resolve finds the matching route, provider@account:model, or combo resolution.
func (r *Router) Resolve(surface string, model string) (core.ResolvedRoute, error) {
	// 0. combo: prefix key form resolution
	if strings.HasPrefix(model, "combo:") {
		remainder := strings.TrimPrefix(model, "combo:")
		if cb, isCombo := r.combos[remainder]; isCombo {
			return r.resolveCombo(surface, cb, "")
		}
		if strings.Contains(remainder, ":") {
			parts := strings.SplitN(remainder, ":", 2)
			if cb, isCombo := r.combos[parts[0]]; isCombo {
				return r.resolveCombo(surface, cb, parts[1])
			}
		}
		// If neither matched a declared combo, fall through to provider resolution
		// (e.g. for a provider literally named "combo")
	}

	// 1. Direct combo resolution by bare combo name
	if cb, isCombo := r.combos[model]; isCombo {
		return r.resolveCombo(surface, cb, "")
	}

	// 2. Direct provider[@account]:model prefix resolution
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
			return r.resolveCombo(surface, cb, targetModel)
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

		// Context-window suffix: a trailing "[1m]" asks for the 1M-token
		// context variant of a model. It is a client-side hint, not part of
		// the upstream model name — strip it before whitelist matching and
		// routing, unless the literal suffixed name is explicitly whitelisted.
		if strings.HasSuffix(targetModel, "[1m]") && !slices.Contains(prov.Models, targetModel) {
			targetModel = strings.TrimSuffix(targetModel, "[1m]")
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

	return core.ResolvedRoute{}, fmt.Errorf("model %q is not a combo and has no provider prefix — use \"provider:model\" or define a combo", model)
}

func (r *Router) expandCombo(surface string, cb config.Combo, visited map[string]bool) ([]core.Hop, error) {
	if visited[cb.Name] {
		return nil, fmt.Errorf("recursive combo reference detected in %q", cb.Name)
	}
	visited[cb.Name] = true

	var hops []core.Hop
	for _, member := range cb.Members {
		if subCb, isSubCombo := r.combos[member]; isSubCombo {
			if !subCb.IsEnabled() {
				continue
			}
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
	if len(hops) == 0 {
		return nil, fmt.Errorf("combo %q has no usable members", cb.Name)
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

// Models returns all concrete model names from provider whitelists and combos
// that resolve successfully on the given surface dialect.
func (r *Router) Models(surface string) []string {
	seen := make(map[string]bool)
	var candidates []string

	// Add combos in key form
	for cbName := range r.combos {
		comboKey := "combo:" + cbName
		if !seen[comboKey] {
			seen[comboKey] = true
			candidates = append(candidates, comboKey)
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

	var models []string
	for _, id := range candidates {
		if _, err := r.Resolve(surface, id); err == nil {
			models = append(models, id)
		}
	}

	sort.Strings(models)
	return models
}
