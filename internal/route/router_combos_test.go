package route

import (
	"strings"
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

	r := New(providers, WithCombos(combos))

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

	// 3. Test model discovery includes combo in key form
	models := r.Models("openai")
	foundCombo := false
	for _, m := range models {
		if m == "combo:smart-combo" {
			foundCombo = true
			break
		}
		if m == "smart-combo" {
			t.Errorf("bare smart-combo must not be returned in Models(), only key form combo:smart-combo")
		}
	}
	if !foundCombo {
		t.Errorf("expected combo:smart-combo in model discovery, got %v", models)
	}
}

func TestRouter_DisabledCombos(t *testing.T) {
	providers := map[string]config.Provider{
		"openai": {
			Dialect: "openai",
			Models:  []string{"gpt-4o", "gpt-4o-mini"},
		},
		"anthropic": {
			Dialect: "openai",
			Models:  []string{"claude-3-5-sonnet"},
		},
	}

	combos := []config.Combo{
		{
			Name:     "disabled-combo",
			Members:  []string{"openai:gpt-4o"},
			Disabled: true,
		},
		{
			Name:     "sub-disabled",
			Members:  []string{"openai:gpt-4o"},
			Disabled: true,
		},
		{
			Name:    "parent-mixed",
			Members: []string{"sub-disabled", "anthropic:claude-3-5-sonnet"},
			Mode:    "ordered",
		},
		{
			Name:    "parent-all-disabled",
			Members: []string{"sub-disabled", "disabled-combo"},
			Mode:    "ordered",
		},
	}

	r := New(providers, WithCombos(combos))

	// 1. Direct disabled error
	_, err := r.Resolve("openai", "disabled-combo")
	if err == nil || err.Error() != `combo "disabled-combo" is disabled` {
		t.Fatalf("expected error `combo \"disabled-combo\" is disabled`, got: %v", err)
	}

	// 2. Provider-position disabled error
	_, err = r.Resolve("openai", "disabled-combo:gpt-4o")
	if err == nil || err.Error() != `combo "disabled-combo" is disabled` {
		t.Fatalf("expected error `combo \"disabled-combo\" is disabled`, got: %v", err)
	}

	// 3. Parent-skip scenario: disabled sub-combo is skipped, remaining member resolves
	res, err := r.Resolve("openai", "parent-mixed")
	if err != nil {
		t.Fatalf("unexpected error resolving parent-mixed: %v", err)
	}
	if len(res.Hops) != 1 || res.Hops[0].Provider != "anthropic" || res.Hops[0].Model != "claude-3-5-sonnet" {
		t.Errorf("expected parent-mixed to resolve to anthropic hop, got: %+v", res.Hops)
	}

	// 4. All-members-disabled failure
	_, err = r.Resolve("openai", "parent-all-disabled")
	if err == nil {
		t.Fatalf("expected error resolving parent-all-disabled, got nil")
	}

	// 5. Models() filters out non-resolvable / disabled combos
	models := r.Models("openai")
	seen := make(map[string]bool)
	for _, m := range models {
		seen[m] = true
	}
	if seen["combo:disabled-combo"] {
		t.Errorf("expected combo:disabled-combo to be filtered out of Models() listing")
	}
	if seen["combo:sub-disabled"] {
		t.Errorf("expected combo:sub-disabled to be filtered out of Models() listing")
	}
	if seen["combo:parent-all-disabled"] {
		t.Errorf("expected combo:parent-all-disabled to be filtered out of Models() listing")
	}
	if !seen["combo:parent-mixed"] {
		t.Errorf("expected combo:parent-mixed (with surviving enabled member) in Models() listing")
	}
	if seen["disabled-combo"] || seen["sub-disabled"] || seen["parent-mixed"] {
		t.Errorf("bare combo names must not appear in Models() listing")
	}
}

func TestRouter_ComboPrefixKeyForm(t *testing.T) {
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
			Dialect: "openai",
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
		{
			Name:    "passthrough-combo",
			Members: []string{"openai:gpt-4o", "anthropic:$model"},
			Mode:    "ordered",
		},
	}

	r := New(providers, WithCombos(combos))

	// 1. combo:<name> resolves identically to bare <name>
	bareRes, err := r.Resolve("openai", "smart-combo")
	if err != nil {
		t.Fatalf("bare combo resolve error: %v", err)
	}
	keyRes, err := r.Resolve("openai", "combo:smart-combo")
	if err != nil {
		t.Fatalf("combo:smart-combo resolve error: %v", err)
	}
	if keyRes.ComboName != bareRes.ComboName || keyRes.Mode != bareRes.Mode || len(keyRes.Hops) != len(bareRes.Hops) {
		t.Errorf("key form resolution mismatch: got %+v, want %+v", keyRes, bareRes)
	}

	// 2. combo:<name>:<model> composes with $model substitution matching <name>:<model>
	barePassRes, err := r.Resolve("openai", "passthrough-combo:claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("bare passthrough resolve error: %v", err)
	}
	keyPassRes, err := r.Resolve("openai", "combo:passthrough-combo:claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("combo:passthrough-combo:claude-3-5-sonnet resolve error: %v", err)
	}
	if keyPassRes.ComboName != barePassRes.ComboName || len(keyPassRes.Hops) != 2 || keyPassRes.Hops[1].Model != "claude-3-5-sonnet" {
		t.Errorf("passthrough key form resolution mismatch: got %+v, want %+v", keyPassRes, barePassRes)
	}

	// 3. combo:<unknown> errors naming the identifier
	_, err = r.Resolve("openai", "combo:unknown-combo")
	if err == nil {
		t.Fatalf("expected error for unknown combo, got nil")
	}
	if !strings.Contains(err.Error(), "combo:unknown-combo") {
		t.Errorf("expected error to name identifier 'combo:unknown-combo', got: %v", err)
	}
}

func TestRouter_ComboPrefixFallthrough(t *testing.T) {
	providers := map[string]config.Provider{
		"combo": {
			Dialect: "openai",
			Models:  []string{"my-model", "shadowed-model"},
		},
		"openai": {
			Dialect: "openai",
			Models:  []string{"gpt-4o"},
		},
	}

	combos := []config.Combo{
		{
			Name:    "shadowed-model",
			Members: []string{"openai:gpt-4o"},
		},
	}

	r := New(providers, WithCombos(combos))

	// 1. combo:<model> resolves to provider "combo" when no combo shadows it
	resProv, err := r.Resolve("openai", "combo:my-model")
	if err != nil {
		t.Fatalf("unexpected error resolving provider combo: %v", err)
	}
	if len(resProv.Hops) != 1 || resProv.Hops[0].Provider != "combo" || resProv.Hops[0].Model != "my-model" {
		t.Errorf("expected hop provider combo model my-model, got: %+v", resProv.Hops)
	}

	// 2. combo:<model> resolves to combo when combo exists (combo takes precedence)
	resCombo, err := r.Resolve("openai", "combo:shadowed-model")
	if err != nil {
		t.Fatalf("unexpected error resolving shadowed combo: %v", err)
	}
	if resCombo.ComboName != "shadowed-model" || len(resCombo.Hops) != 1 || resCombo.Hops[0].Provider != "openai" {
		t.Errorf("expected combo shadowed-model to take precedence over provider combo, got: %+v", resCombo)
	}
}
