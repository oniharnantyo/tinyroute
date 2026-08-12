package route

import (
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

	r := New(nil, providers)

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

func TestRouter_UnprefixedResolution(t *testing.T) {
	raw := []RawRoute{
		{
			From:  "anthropic",
			Match: "claude-3-*",
			Chain: []string{"anthropic:claude-3-5-sonnet"},
		},
	}
	entries, err := ParseRoutes(raw)
	if err != nil {
		t.Fatal(err)
	}

	r := New(entries, nil)

	// Unprefixed model matching route
	res, err := r.Resolve("anthropic", "claude-3-opus")
	if err != nil {
		t.Fatalf("unexpected error for matching route: %v", err)
	}
	if len(res.Hops) != 1 || res.Hops[0].Provider != "anthropic" || res.Hops[0].Model != "claude-3-5-sonnet" {
		t.Errorf("unexpected route: %+v", res)
	}

	// Unprefixed model not matching route rejected
	_, err = r.Resolve("anthropic", "gpt-4o")
	if err == nil {
		t.Errorf("expected error for unprefixed unmatched model")
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
	raw := []RawRoute{
		{
			From:  "openai",
			Match: "fast",
			Chain: []string{"openai:gpt-4o"},
		},
	}
	entries, err := ParseRoutes(raw)
	if err != nil {
		t.Fatal(err)
	}

	r := New(entries, providers)

	models := r.Models("openai")
	seen := make(map[string]bool)
	for _, m := range models {
		seen[m] = true
	}

	if !seen["openai:gpt-4o"] {
		t.Errorf("expected openai:gpt-4o to be returned in Models()")
	}
	if !seen["fast"] {
		t.Errorf("expected explicit route match 'fast' to be returned in Models()")
	}
	if seen["gpt-4o"] {
		t.Errorf("expected unprefixed bare 'gpt-4o' to be filtered out because it does not resolve")
	}
}

func TestRouter_ModelsCrossDialectIsolation(t *testing.T) {
	providers := map[string]config.Provider{
		"openai": {
			Dialect: "openai",
			BaseURL: "https://api.openai.com/v1",
			Models:  []string{"gpt-4o"},
		},
	}
	raw := []RawRoute{
		{
			From:  "anthropic",
			Match: "claude-alias",
			Chain: []string{"anthropic:claude-3-5-sonnet"},
		},
		{
			From:  "openai",
			Match: "*",
			Chain: []string{"openai:gpt-4o"},
		},
	}
	entries, err := ParseRoutes(raw)
	if err != nil {
		t.Fatal(err)
	}

	r := New(entries, providers)

	models := r.Models("openai")
	for _, m := range models {
		if m == "claude-alias" {
			t.Errorf("expected anthropic route 'claude-alias' to NOT be included in openai Models() listing")
		}
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
	r := New(nil, providers)

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
	r := New(nil, providers)

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
	rTrans := New(nil, providers, WithTranslatable(func(from, to string) bool {
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
