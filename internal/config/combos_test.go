package config

import (
	"reflect"
	"testing"
)

func TestConfig_ValidateComboName(t *testing.T) {
	existing := []Combo{
		{Name: "existing-combo"},
	}

	if err := ValidateComboName("valid-name_1", existing); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
	if err := ValidateComboName("", existing); err == nil {
		t.Errorf("expected empty error")
	}
	if err := ValidateComboName("foo:bar", existing); err == nil {
		t.Errorf("expected colon error")
	}
	if err := ValidateComboName("invalid!name", existing); err == nil {
		t.Errorf("expected charset error")
	}
	if err := ValidateComboName("existing-combo", existing); err == nil {
		t.Errorf("expected duplicate error")
	}
	if err := ValidateComboName("combo", existing); err == nil {
		t.Errorf("expected error for combo name 'combo'")
	}
}

func TestConfig_GetMemberCandidates(t *testing.T) {
	topo := Topology{
		Providers: map[string]Provider{
			"p1": {Models: []string{"m1", "m2"}},
			"p2": {Models: []string{"m3"}, Accounts: []Account{{Name: "single"}}},
			"p3": {
				Models: []string{"m4", "m5"},
				Accounts: []Account{
					{Name: "work"},
					{Name: "jane@example.com"},
				},
			},
			"p4": {Models: []string{}},
		},
	}
	cands := GetMemberCandidates(topo)
	expected := []string{
		"p1:m1", "p1:m2",
		"p2:m3",
		"p3:m4", "p3:m5",
		"p3@jane@example.com:m4", "p3@jane@example.com:m5",
		"p3@work:m4", "p3@work:m5",
	}
	if !reflect.DeepEqual(cands, expected) {
		t.Errorf("expected %v, got %v", expected, cands)
	}

	emptyCands := GetMemberCandidates(Topology{})
	if len(emptyCands) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(emptyCands))
	}
}

func TestConfig_GetModelCandidates(t *testing.T) {
	topo := Topology{
		Providers: map[string]Provider{
			"p1": {Models: []string{"m1", "m2"}},
			"p2": {Models: []string{"m3"}, Accounts: []Account{{Name: "single"}}},
			"p3": {
				Models: []string{"m4", "m5"},
				Accounts: []Account{
					{Name: "work"},
					{Name: "personal"},
				},
			},
			"p4": {Models: []string{}},
		},
	}
	models := GetModelCandidates(topo)
	expected := []string{
		"p1:m1", "p1:m2",
		"p2:m3",
		"p3:m4", "p3:m5",
	}
	if !reflect.DeepEqual(models, expected) {
		t.Errorf("expected %v, got %v", expected, models)
	}

	emptyModels := GetModelCandidates(Topology{})
	if len(emptyModels) != 0 {
		t.Errorf("expected 0 models, got %d", len(emptyModels))
	}
}

func TestConfig_GetAccountOptions(t *testing.T) {
	topo := Topology{
		Providers: map[string]Provider{
			"p1": {Models: []string{"m1", "m2"}},
			"p2": {Models: []string{"m3"}, Accounts: []Account{{Name: "single"}}},
			"p3": {
				Models: []string{"m4"},
				Accounts: []Account{
					{Name: "work"},
					{Name: "personal"},
				},
			},
			"p4": {
				Models: []string{"m5"},
				Accounts: []Account{
					{Name: "team-a"},
					{Name: "team-b"},
				},
			},
		},
	}
	opts := GetAccountOptions(topo)
	expected := []string{
		"p3@personal", "p3@work",
		"p4@team-a", "p4@team-b",
	}
	if !reflect.DeepEqual(opts, expected) {
		t.Errorf("expected %v, got %v", expected, opts)
	}

	emptyOpts := GetAccountOptions(Topology{})
	if len(emptyOpts) != 0 {
		t.Errorf("expected 0 options, got %d", len(emptyOpts))
	}
}

func TestConfig_RenameComboAccount(t *testing.T) {
	combos := []Combo{
		{
			Name:    "combo1",
			Members: []string{"glm@old:m1", "anthropic:m2"},
		},
		{
			Name:    "combo2",
			Members: []string{"openai:m1", "glm:m2"},
		},
	}
	renamed := RenameComboAccount(combos, "glm", "old", "new")
	if len(renamed) != 2 {
		t.Fatalf("expected 2 combos, got %d", len(renamed))
	}
	if renamed[0].Members[0] != "glm@new:m1" || renamed[0].Members[1] != "anthropic:m2" {
		t.Errorf("expected glm@new:m1, got %v", renamed[0].Members)
	}
	if !reflect.DeepEqual(renamed[1].Members, combos[1].Members) {
		t.Errorf("untouched combo modified: %v", renamed[1].Members)
	}
}

func TestConfig_DowngradeComboAccount(t *testing.T) {
	combos := []Combo{
		{
			Name:    "c1-downgrade-clean",
			Members: []string{"glm@work:m1", "anthropic:m2"},
		},
		{
			Name:    "c2-downgrade-dedup",
			Members: []string{"glm:m1", "glm@work:m1", "anthropic:m2"},
		},
		{
			Name:    "c3-dedup-to-single",
			Members: []string{"glm@work:m1", "glm:m1"},
		},
		{
			Name:    "c4-both-pinned-same-model",
			Members: []string{"glm@work:m1", "glm@work:m1"},
		},
		{
			Name:    "c5-untouched",
			Members: []string{"openai:m1", "anthropic:m2"},
		},
	}

	updated, modified := DowngradeComboAccount(combos, "glm", "work")

	expectedModified := []string{"c1-downgrade-clean", "c2-downgrade-dedup", "c3-dedup-to-single", "c4-both-pinned-same-model"}
	if !reflect.DeepEqual(modified, expectedModified) {
		t.Errorf("expected modified %v, got %v", expectedModified, modified)
	}

	if len(updated) != 5 {
		t.Fatalf("expected all 5 combos to survive downgrade, got %d", len(updated))
	}
	if !reflect.DeepEqual(updated[0].Members, []string{"glm:m1", "anthropic:m2"}) {
		t.Errorf("c1 members unexpected: %v", updated[0].Members)
	}
	if !reflect.DeepEqual(updated[1].Members, []string{"glm:m1", "anthropic:m2"}) {
		t.Errorf("c2 members unexpected: %v", updated[1].Members)
	}
	if !reflect.DeepEqual(updated[2].Members, []string{"glm:m1"}) {
		t.Errorf("c3 members unexpected: %v", updated[2].Members)
	}
	if !reflect.DeepEqual(updated[3].Members, []string{"glm:m1"}) {
		t.Errorf("c4 members unexpected: %v", updated[3].Members)
	}
	if !reflect.DeepEqual(updated[4].Members, []string{"openai:m1", "anthropic:m2"}) {
		t.Errorf("c5 members unexpected: %v", updated[4].Members)
	}
}

func TestConfig_SplitAndTrim(t *testing.T) {
	res := SplitAndTrim(" a , b,  , c  ")
	expected := []string{"a", "b", "c"}
	if !reflect.DeepEqual(res, expected) {
		t.Errorf("expected %v, got %v", expected, res)
	}
	if len(SplitAndTrim("")) != 0 {
		t.Errorf("expected empty for empty string")
	}
}
