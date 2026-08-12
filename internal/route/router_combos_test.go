package route

import (
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/config"
)

func TestRouter_AccountPinnedAndCombos(t *testing.T) {
	providers := map[string]config.Provider{
		"openai": {
			Dialect: "openai",
			Accounts: []config.Account{
				{Name: "acc1", APIKey: "sk-1"},
				{Name: "acc2", APIKey: "sk-2"},
			},
			Models: []string{"gpt-4o", "gpt-4o-mini"},
		},
		"anthropic": {
			Dialect: "openai", // same dialect for test
			Models:  []string{"claude-3-5-sonnet"},
		},
	}

	combos := []config.Combo{
		{
			Name:         "smart-combo",
			Members:      []string{"openai@acc1:gpt-4o", "openai@acc2:gpt-4o-mini"},
			Mode:         "fused",
			Capabilities: []string{"vision"},
		},
	}

	r := New(nil, providers, WithCombos(combos))

	// 1. Test pinned account resolution
	resPinned, err := r.Resolve("openai", "openai@acc1:gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error resolving pinned account: %v", err)
	}
	if len(resPinned.Hops) != 1 || resPinned.Hops[0].Account != "acc1" || resPinned.Hops[0].Model != "gpt-4o" {
		t.Errorf("expected hop with account acc1, got %v", resPinned.Hops)
	}

	// 2. Test combo resolution
	resCombo, err := r.Resolve("openai", "smart-combo")
	if err != nil {
		t.Fatalf("unexpected error resolving combo: %v", err)
	}
	if resCombo.ComboName != "smart-combo" || resCombo.Mode != "fused" {
		t.Errorf("expected combo smart-combo mode fused, got name %s mode %s", resCombo.ComboName, resCombo.Mode)
	}
	if len(resCombo.Hops) != 2 {
		t.Fatalf("expected 2 hops in combo, got %d", len(resCombo.Hops))
	}
	if resCombo.Hops[0].Account != "acc1" || resCombo.Hops[1].Account != "acc2" {
		t.Errorf("expected hops for acc1 and acc2, got %v", resCombo.Hops)
	}

	// 3. Test model discovery includes combo
	models := r.Models("openai")
	foundCombo := false
	for _, m := range models {
		if m == "smart-combo" {
			foundCombo = true
			break
		}
	}
	if !foundCombo {
		t.Errorf("expected smart-combo in model discovery, got %v", models)
	}
}
