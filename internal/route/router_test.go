package route

import (
	"strings"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/config"
)

func TestRouter_DirectPrefixResolution(t *testing.T) {
	providers := map[string]config.Provider{
		"openai": {
			Dialect: "openai",
			BaseURL: "https://api.openai.com/v1",
			Models:  []string{"gpt-4o", "gpt-4o-mini"},
		},
		"anthropic": {
			Dialect: "anthropic",
			BaseURL: "https://api.anthropic.com",
		},
	}

	r := New(providers)

	// Valid whitelisted model
	res, err := r.Resolve("openai", "openai:gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error resolving openai:gpt-4o: %v", err)
	}
	if len(res.Hops) != 1 || res.Hops[0].Provider != "openai" || res.Hops[0].Model != "gpt-4o" {
		t.Errorf("unexpected resolved route: %+v", res)
	}

	// Non-whitelisted model for provider with whitelist
	_, err = r.Resolve("openai", "openai:gpt-3.5-turbo")
	if err == nil {
		t.Errorf("expected error for non-whitelisted model, got nil")
	}

	// Provider with empty whitelist allows any model
	res, err = r.Resolve("anthropic", "anthropic:claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("unexpected error resolving anthropic:claude-3-5-sonnet: %v", err)
	}
	if len(res.Hops) != 1 || res.Hops[0].Provider != "anthropic" || res.Hops[0].Model != "claude-3-5-sonnet" {
		t.Errorf("unexpected resolved route: %+v", res)
	}

	// Unknown provider prefix
	_, err = r.Resolve("unknown", "unknown:model-x")
	if err == nil {
		t.Errorf("expected error for unknown provider prefix")
	}
}

func TestRouter_ContextWindowSuffixStripped(t *testing.T) {
	providers := map[string]config.Provider{
		"anthropic": {
			Dialect: "anthropic",
			BaseURL: "https://api.anthropic.com",
			Models:  []string{"claude-sonnet-4-5"},
		},
	}

	r := New(providers)

	// Suffix variant resolves to the base whitelisted model
	res, err := r.Resolve("anthropic", "anthropic:claude-sonnet-4-5[1m]")
	if err != nil {
		t.Fatalf("unexpected error resolving anthropic:claude-sonnet-4-5[1m]: %v", err)
	}
	if len(res.Hops) != 1 || res.Hops[0].Model != "claude-sonnet-4-5" {
		t.Errorf("expected [1m] suffix to be stripped before upstream routing, got: %+v", res)
	}

	// Provider with empty whitelist also strips the suffix
	open := map[string]config.Provider{
		"any": {Dialect: "anthropic", BaseURL: "https://example.com"},
	}
	rOpen := New(open)
	res, err = rOpen.Resolve("anthropic", "any:claude-sonnet-4-5[1m]")
	if err != nil {
		t.Fatalf("unexpected error resolving suffix model on open whitelist: %v", err)
	}
	if res.Hops[0].Model != "claude-sonnet-4-5" {
		t.Errorf("expected suffix stripped on open whitelist, got: %+v", res)
	}

	// A literal whitelisted name containing the suffix is preserved verbatim
	literal := map[string]config.Provider{
		"anthropic": {
			Dialect: "anthropic",
			BaseURL: "https://api.anthropic.com",
			Models:  []string{"claude-sonnet-4-5[1m]"},
		},
	}
	rLit := New(literal)
	res, err = rLit.Resolve("anthropic", "anthropic:claude-sonnet-4-5[1m]")
	if err != nil {
		t.Fatalf("unexpected error resolving literal suffixed whitelist entry: %v", err)
	}
	if res.Hops[0].Model != "claude-sonnet-4-5[1m]" {
		t.Errorf("expected literal whitelisted name preserved, got: %+v", res)
	}

	// Base model still resolves normally
	_, err = r.Resolve("anthropic", "anthropic:claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("unexpected error resolving base model: %v", err)
	}
}

func TestRouter_UnprefixedModelError_NoRouteReference(t *testing.T) {
	providers := map[string]config.Provider{
		"anthropic": {
			Dialect: "anthropic",
			BaseURL: "https://api.anthropic.com",
			Models:  []string{"claude-sonnet-4-5"},
		},
	}
	r := New(providers)

	// 1. Bare model name that is not a combo fails with unprefixed non-combo error
	_, err := r.Resolve("anthropic", "claude-sonnet-4-5")
	if err == nil {
		t.Fatalf("expected error for unprefixed non-combo model, got nil")
	}
	expectedMsg := `model "claude-sonnet-4-5" is not a combo and has no provider prefix — use "provider:model" or define a combo`
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
	}
	if strings.Contains(strings.ToLower(err.Error()), "route") {
		t.Errorf("error message must not mention 'route': %s", err.Error())
	}

	// 2. Glob-style bare names fail similarly
	_, err = r.Resolve("anthropic", "claude-3-opus")
	if err == nil {
		t.Fatalf("expected error for glob bare model name, got nil")
	}
	if strings.Contains(strings.ToLower(err.Error()), "route") {
		t.Errorf("error message must not mention 'route': %s", err.Error())
	}
}

func TestRouter_ModelsFiltering(t *testing.T) {
	providers := map[string]config.Provider{
		"openai": {
			Dialect: "openai",
			BaseURL: "https://api.openai.com/v1",
			Models:  []string{"gpt-4o"},
		},
	}

	r := New(providers)

	models := r.Models("openai")
	seen := make(map[string]bool)
	for _, m := range models {
		seen[m] = true
	}

	if !seen["openai:gpt-4o"] {
		t.Errorf("expected openai:gpt-4o to be returned in Models()")
	}
	if seen["fast"] {
		t.Errorf("expected no route matches like 'fast' to be returned")
	}
	if seen["gpt-4o"] {
		t.Errorf("expected unprefixed bare 'gpt-4o' to be filtered out because it does not resolve")
	}
}

func TestRouter_ModelsDeterministicOrdering(t *testing.T) {
	providers := map[string]config.Provider{
		"openai": {
			Dialect: "openai",
			Models:  []string{"gpt-4o", "gpt-4o-mini"},
		},
		"anthropic": {
			Dialect: "anthropic",
			Models:  []string{"claude-3-5-sonnet"},
		},
	}
	r := New(providers)

	first := r.Models("openai")
	for i := 0; i < 10; i++ {
		next := r.Models("openai")
		if len(first) != len(next) {
			t.Fatalf("model count mismatch: %d vs %d", len(first), len(next))
		}
	}
}

func TestRouter_FaithfulSurfaceGuard(t *testing.T) {
	providers := map[string]config.Provider{
		"openai": {
			Dialect: "openai",
			BaseURL: "https://api.openai.com/v1",
			Models:  []string{"gpt-4o"},
		},
		"anthropic": {
			Dialect: "anthropic",
			BaseURL: "https://api.anthropic.com",
			Models:  []string{"claude-3-5-sonnet"},
		},
	}
	r := New(providers)

	// Same-dialect prefix resolves on openai surface
	res, err := r.Resolve("openai", "openai:gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error resolving openai:gpt-4o on openai surface: %v", err)
	}
	if len(res.Hops) != 1 || res.Hops[0].Provider != "openai" {
		t.Errorf("unexpected resolved route: %+v", res)
	}

	// Cross-dialect prefix is rejected on openai surface when translatable is false (default)
	_, err = r.Resolve("openai", "anthropic:claude-3-5-sonnet")
	if err == nil {
		t.Fatalf("expected cross-dialect resolution to be rejected, got nil error")
	}

	// When translatable predicate returns true for (anthropic, openai), cross-dialect hop resolves
	rTrans := New(providers, WithTranslatable(func(from, to string) bool {
		return (from == "anthropic" && to == "openai") || (from == "openai" && to == "anthropic")
	}))

	resCross, err := rTrans.Resolve("anthropic", "openai:gpt-4o")
	if err != nil {
		t.Fatalf("expected translatable cross-dialect resolution to succeed, got: %v", err)
	}
	if len(resCross.Hops) != 1 || resCross.Hops[0].Provider != "openai" {
		t.Errorf("unexpected cross-dialect resolved route: %+v", resCross)
	}

	// Models("anthropic") with translatable predicate now surfaces OpenAI models
	anthropicModels := rTrans.Models("anthropic")
	seenAnthropic := make(map[string]bool)
	for _, m := range anthropicModels {
		seenAnthropic[m] = true
	}
	if !seenAnthropic["openai:gpt-4o"] {
		t.Errorf("expected openai:gpt-4o in Models('anthropic') when translatable is true")
	}

	// Models("openai") returns only OpenAI resolvable IDs
	models := r.Models("openai")
	seen := make(map[string]bool)
	for _, m := range models {
		seen[m] = true
	}
	if !seen["openai:gpt-4o"] {
		t.Errorf("expected openai:gpt-4o in Models('openai')")
	}
	if seen["anthropic:claude-3-5-sonnet"] {
		t.Errorf("expected anthropic:claude-3-5-sonnet to NOT be in Models('openai')")
	}
}

func TestRouter_Models_EveryListedComboResolves(t *testing.T) {
	providers := map[string]config.Provider{
		"openai": {
			Dialect: "openai",
			Models:  []string{"gpt-4o"},
		},
	}
	combos := []config.Combo{
		{
			Name:    "my-combo",
			Members: []string{"openai:gpt-4o"},
		},
	}
	r := New(providers, WithCombos(combos))
	models := r.Models("openai")

	foundCombo := false
	for _, id := range models {
		if strings.HasPrefix(id, "combo:") {
			foundCombo = true
			if _, err := r.Resolve("openai", id); err != nil {
				t.Errorf("listed combo model %q failed to resolve: %v", id, err)
			}
		}
	}
	if !foundCombo {
		t.Errorf("expected combo:my-combo in Models('openai')")
	}
}
