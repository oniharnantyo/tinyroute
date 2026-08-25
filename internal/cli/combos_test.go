package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/cli/interactive"
	"github.com/oniharnantyo/tinyroute/internal/config"
)

func TestGetMemberCandidates(t *testing.T) {
	t.Run("empty topology returns empty slice", func(t *testing.T) {
		topo := config.Topology{}
		candidates := GetMemberCandidates(topo)
		if len(candidates) != 0 {
			t.Fatalf("expected 0 candidates, got %v", candidates)
		}
	})

	t.Run("providers without whitelists are skipped", func(t *testing.T) {
		topo := config.Topology{
			Providers: map[string]config.Provider{
				"emptyprov": {
					Dialect: "openai",
					BaseURL: "http://localhost:8000",
				},
				"anthropic": {
					Dialect: "anthropic",
					BaseURL: "https://api.anthropic.com",
					Models:  []string{"claude-sonnet-4.5", "claude-haiku-3.5"},
				},
				"openai": {
					Dialect: "openai",
					BaseURL: "https://api.openai.com/v1",
					Models:  []string{"gpt-5.2"},
				},
			},
		}

		candidates := GetMemberCandidates(topo)
		expected := []string{
			"anthropic:claude-sonnet-4.5",
			"anthropic:claude-haiku-3.5",
			"openai:gpt-5.2",
		}

		if len(candidates) != len(expected) {
			t.Fatalf("expected %d candidates, got %d (%v)", len(expected), len(candidates), candidates)
		}
		for i, exp := range expected {
			if candidates[i] != exp {
				t.Errorf("candidate %d: expected %q, got %q", i, exp, candidates[i])
			}
		}
	})
}

func TestValidateComboName(t *testing.T) {
	existing := []config.Combo{
		{Name: "coding-fast"},
	}

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "valid name",
			input:   "my-combo_1.0",
			wantErr: "",
		},
		{
			name:    "empty name",
			input:   "   ",
			wantErr: "combo name cannot be empty",
		},
		{
			name:    "contains colon",
			input:   "openai:gpt-4",
			wantErr: "cannot contain ':'",
		},
		{
			name:    "invalid characters",
			input:   "combo@special!",
			wantErr: "can only contain letters, numbers",
		},
		{
			name:    "duplicate name",
			input:   "coding-fast",
			wantErr: "already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateComboName(tt.input, existing)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestAddComboCore(t *testing.T) {
	baseTopo := config.Topology{
		Providers: map[string]config.Provider{
			"anthropic": {
				Dialect: "anthropic",
				BaseURL: "https://api.anthropic.com",
				Models:  []string{"claude-sonnet-4.5"},
			},
			"openai": {
				Dialect: "openai",
				BaseURL: "https://api.openai.com/v1",
				Models:  []string{"gpt-5.2"},
			},
		},
		Combos: []config.Combo{
			{
				Name:    "existing-combo",
				Members: []string{"anthropic:claude-sonnet-4.5", "openai:gpt-5.2"},
				Mode:    "ordered",
			},
		},
	}

	t.Run("valid combo adds successfully", func(t *testing.T) {
		topo, err := AddComboCore(baseTopo, "new-combo", []string{"anthropic:claude-sonnet-4.5", "openai:gpt-5.2"}, "pool", []string{"vision", "pdf"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(topo.Combos) != 2 {
			t.Fatalf("expected 2 combos, got %d", len(topo.Combos))
		}
		added := topo.Combos[1]
		if added.Name != "new-combo" || added.Mode != "pool" || len(added.Members) != 2 || len(added.Capabilities) != 2 {
			t.Errorf("unexpected combo data: %+v", added)
		}
	})

	t.Run("duplicate name rejected", func(t *testing.T) {
		_, err := AddComboCore(baseTopo, "existing-combo", []string{"anthropic:claude-sonnet-4.5", "openai:gpt-5.2"}, "ordered", nil)
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("expected already exists error, got %v", err)
		}
	})

	t.Run("invalid mode rejected", func(t *testing.T) {
		_, err := AddComboCore(baseTopo, "invalid-mode", []string{"anthropic:claude-sonnet-4.5", "openai:gpt-5.2"}, "roundrobin", nil)
		if err == nil || !strings.Contains(err.Error(), "invalid mode") {
			t.Fatalf("expected invalid mode error, got %v", err)
		}
	})

	t.Run("single member combo adds successfully", func(t *testing.T) {
		topo, err := AddComboCore(baseTopo, "one-member", []string{"anthropic:claude-sonnet-4.5"}, "ordered", nil)
		if err != nil {
			t.Fatalf("unexpected error for single-member combo: %v", err)
		}
		if len(topo.Combos) != 2 || len(topo.Combos[1].Members) != 1 {
			t.Fatalf("expected 1-member combo appended, got %+v", topo.Combos)
		}
	})

	t.Run("no members rejected", func(t *testing.T) {
		_, err := AddComboCore(baseTopo, "no-members", nil, "ordered", nil)
		if err == nil || !strings.Contains(err.Error(), "at least 1 member") {
			t.Fatalf("expected at least 1 member error, got %v", err)
		}
	})

	t.Run("empty member string rejected", func(t *testing.T) {
		_, err := AddComboCore(baseTopo, "empty-member", []string{"anthropic:claude-sonnet-4.5", "  "}, "ordered", nil)
		if err == nil || !strings.Contains(err.Error(), "member cannot be empty") {
			t.Fatalf("expected member cannot be empty error, got %v", err)
		}
	})
}

func TestCmdCombosAdd_TypedArgs(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	configPath := filepath.Join(dotDir, "config.yaml")
	initTopo := config.Topology{
		Providers: map[string]config.Provider{
			"anthropic": {
				Dialect: "anthropic",
				BaseURL: "https://api.anthropic.com",
				Models:  []string{"claude-sonnet-4.5"},
			},
			"glm": {
				Dialect: "openai",
				BaseURL: "https://open.bigmodel.cn/api/paas/v4",
				Models:  []string{"glm-4.7"},
			},
		},
	}
	if err := config.WriteTopology(configPath, initTopo); err != nil {
		t.Fatalf("write init topology: %v", err)
	}

	falseVal := false
	interactive.SetCanPromptOverride(&falseVal)
	defer interactive.SetCanPromptOverride(nil)

	// Typed args bypass all prompts
	err := cmdCombosAdd([]string{"coding", "--members=anthropic:claude-sonnet-4.5,glm:glm-4.7", "--mode=ordered", "--capabilities=vision,pdf"})
	if err != nil {
		t.Fatalf("cmdCombosAdd typed args error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	topo, err := config.ParseTopology(data)
	if err != nil {
		t.Fatalf("parse topology: %v", err)
	}

	if len(topo.Combos) != 1 {
		t.Fatalf("expected 1 combo, got %d", len(topo.Combos))
	}
	cb := topo.Combos[0]
	if cb.Name != "coding" || cb.Mode != "ordered" {
		t.Errorf("unexpected combo: %+v", cb)
	}
	if len(cb.Members) != 2 || cb.Members[0] != "anthropic:claude-sonnet-4.5" || cb.Members[1] != "glm:glm-4.7" {
		t.Errorf("unexpected members order: %v", cb.Members)
	}
	if len(cb.Capabilities) != 2 || cb.Capabilities[0] != "vision" || cb.Capabilities[1] != "pdf" {
		t.Errorf("unexpected capabilities: %v", cb.Capabilities)
	}
}

func TestCmdCombosAdd_EdgeStates(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(dotDir, "config.yaml")

	falseVal := false
	interactive.SetCanPromptOverride(&falseVal)
	defer interactive.SetCanPromptOverride(nil)

	t.Run("no providers configured informational exit", func(t *testing.T) {
		emptyTopo := config.Topology{}
		if err := config.WriteTopology(configPath, emptyTopo); err != nil {
			t.Fatalf("write topo: %v", err)
		}

		err := cmdCombosAdd([]string{})
		if err != nil {
			t.Fatalf("expected nil error on informational exit, got %v", err)
		}
	})

	t.Run("non-TTY with missing args returns clear usage error", func(t *testing.T) {
		topoWithProviders := config.Topology{
			Providers: map[string]config.Provider{
				"anthropic": {
					Dialect: "anthropic",
					BaseURL: "https://api.anthropic.com",
					Models:  []string{"claude-sonnet-4.5"},
				},
				"openai": {
					Dialect: "openai",
					BaseURL: "https://api.openai.com/v1",
					Models:  []string{"gpt-5.2"},
				},
			},
		}
		if err := config.WriteTopology(configPath, topoWithProviders); err != nil {
			t.Fatalf("write topo: %v", err)
		}

		err := cmdCombosAdd([]string{})
		if err == nil {
			t.Fatalf("expected error when missing args in non-TTY, got nil")
		}
		if !strings.Contains(err.Error(), "missing required arguments; usage:") {
			t.Errorf("expected usage message in error, got %v", err)
		}
	})
}

func TestCmdCombosList_And_Remove(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(dotDir, "config.yaml")

	topo := config.Topology{
		Providers: map[string]config.Provider{
			"anthropic": {
				Dialect: "anthropic",
				BaseURL: "https://api.anthropic.com",
				Models:  []string{"claude-sonnet-4.5"},
			},
		},
		Combos: []config.Combo{
			{
				Name:         "test-combo",
				Members:      []string{"anthropic:claude-sonnet-4.5"},
				Mode:         "ordered",
				Capabilities: []string{"vision"},
			},
		},
	}
	if err := config.WriteTopology(configPath, topo); err != nil {
		t.Fatalf("write topo: %v", err)
	}

	// List
	if err := cmdCombosList([]string{}); err != nil {
		t.Fatalf("cmdCombosList error: %v", err)
	}

	// Remove nonexistent combo
	if err := cmdCombosRemove([]string{"nonexistent"}); err == nil {
		t.Fatalf("expected error removing nonexistent combo, got nil")
	}

	// Remove with --force
	if err := cmdCombosRemove([]string{"test-combo", "--force"}); err != nil {
		t.Fatalf("cmdCombosRemove error: %v", err)
	}

	// List empty
	if err := cmdCombosList([]string{}); err != nil {
		t.Fatalf("cmdCombosList empty error: %v", err)
	}

	// Verify command builder
	cmd := cmdCombos()
	if cmd.Name != "combos" || len(cmd.Commands) != 3 {
		t.Errorf("unexpected cmdCombos spec: %+v", cmd)
	}
}

func TestRunComboAddWizard_NonInteractive(t *testing.T) {
	topo := config.Topology{
		Providers: map[string]config.Provider{
			"anthropic": {
				Dialect: "anthropic",
				BaseURL: "https://api.anthropic.com",
				Models:  []string{"claude-sonnet-4.5", "claude-haiku-3.5"},
			},
		},
	}

	falseVal := false
	interactive.SetCanPromptOverride(&falseVal)
	defer interactive.SetCanPromptOverride(nil)

	name, members, mode, caps, err := RunComboAddWizard(topo, "smart-combo")
	if err != nil {
		t.Fatalf("unexpected wizard error: %v", err)
	}
	if name != "smart-combo" {
		t.Errorf("expected name 'smart-combo', got %q", name)
	}
	if len(members) < 1 {
		t.Errorf("expected at least 1 member collected, got %v", members)
	}
	if mode != "ordered" {
		t.Errorf("expected mode 'ordered', got %q", mode)
	}
	_ = caps
}

func TestRunComboAddWizard_InteractiveCustomFlow(t *testing.T) {
	topo := config.Topology{
		Providers: map[string]config.Provider{
			"anthropic": {
				Dialect: "anthropic",
				BaseURL: "https://api.anthropic.com",
				Models:  []string{"claude-sonnet-4.5", "claude-haiku-3.5"},
			},
			"openai": {
				Dialect: "openai",
				BaseURL: "https://api.openai.com/v1",
				Models:  []string{"gpt-5.2"},
			},
		},
		Combos: []config.Combo{
			{Name: "existing-combo"},
		},
	}

	trueVal := true
	interactive.SetCanPromptOverride(&trueVal)
	defer interactive.SetCanPromptOverride(nil)

	// Test 1: Name validation re-prompting & pick sequence
	inputCalls := 0
	interactive.SetInputOverride(func(message, defaultVal string, validator func(string) error) (string, error) {
		inputCalls++
		if inputCalls == 1 {
			// First attempt: duplicate name
			if err := validator("existing-combo"); err == nil {
				t.Errorf("expected validator to reject duplicate name")
			}
		}
		// Second attempt: valid unique name
		return "fast-coding", nil
	})
	defer interactive.SetInputOverride(nil)

	selectCalls := 0
	interactive.SetSelectOverride(func(message string, options []string) (string, error) {
		selectCalls++
		if selectCalls == 1 {
			// Step 2, Member 1: Verify "Done" option is not present
			for _, opt := range options {
				if strings.Contains(opt, "Done") {
					t.Errorf("unexpected Done option on member 1: %v", options)
				}
			}
			return "openai:gpt-5.2", nil
		}
		if selectCalls == 2 {
			// Step 2, Member 2: candidate openai:gpt-5.2 is excluded, and the
			// completion option is available once one member is chosen
			hasDone := false
			for _, opt := range options {
				if opt == "openai:gpt-5.2" {
					t.Errorf("candidate openai:gpt-5.2 should be excluded from second select")
				}
				if strings.Contains(opt, "Done") {
					hasDone = true
				}
			}
			if !hasDone {
				t.Errorf("expected Done option on member 2: %v", options)
			}
			return "anthropic:claude-sonnet-4.5", nil
		}
		if selectCalls == 3 {
			return "Done — enough members", nil
		}
		if selectCalls == 4 {
			// Step 3: Mode selection
			return "pool - load-balance requests across healthy members", nil
		}
		return options[0], nil
	})
	defer interactive.SetSelectOverride(nil)

	interactive.SetMultiSelectOverride(func(message string, options []string) ([]string, error) {
		return []string{"vision", "pdf"}, nil
	})
	defer interactive.SetMultiSelectOverride(nil)

	interactive.SetConfirmOverride(func(message string, defaultVal bool) (bool, error) {
		return true, nil
	})
	defer interactive.SetConfirmOverride(nil)

	name, members, mode, caps, err := RunComboAddWizard(topo, "")
	if err != nil {
		t.Fatalf("unexpected wizard error: %v", err)
	}
	if name != "fast-coding" {
		t.Errorf("expected name 'fast-coding', got %q", name)
	}
	if len(members) != 2 || members[0] != "openai:gpt-5.2" || members[1] != "anthropic:claude-sonnet-4.5" {
		t.Errorf("unexpected members order: %v", members)
	}
	if mode != "pool" {
		t.Errorf("expected mode 'pool', got %q", mode)
	}
	if len(caps) != 2 || caps[0] != "vision" || caps[1] != "pdf" {
		t.Errorf("unexpected capabilities: %v", caps)
	}
}

func TestCmdCombosAdd_InteractiveWizardFullExecution(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(dotDir, "config.yaml")

	topo := config.Topology{
		Providers: map[string]config.Provider{
			"anthropic": {
				Dialect: "anthropic",
				BaseURL: "https://api.anthropic.com",
				Models:  []string{"claude-sonnet-4.5", "claude-haiku-3.5"},
			},
		},
	}
	if err := config.WriteTopology(configPath, topo); err != nil {
		t.Fatalf("write topo: %v", err)
	}

	trueVal := true
	interactive.SetCanPromptOverride(&trueVal)
	defer interactive.SetCanPromptOverride(nil)

	interactive.SetInputOverride(func(message, defaultVal string, validator func(string) error) (string, error) {
		return "wizard-combo", nil
	})
	defer interactive.SetInputOverride(nil)

	selectCall := 0
	interactive.SetSelectOverride(func(message string, options []string) (string, error) {
		selectCall++
		if selectCall == 1 {
			return "anthropic:claude-sonnet-4.5", nil
		}
		if selectCall == 2 {
			return "anthropic:claude-haiku-3.5", nil
		}
		if selectCall == 3 {
			return "Done — enough members", nil
		}
		if selectCall == 4 {
			return "fused - combine responses from multiple models", nil
		}
		return options[0], nil
	})
	defer interactive.SetSelectOverride(nil)

	interactive.SetMultiSelectOverride(func(message string, options []string) ([]string, error) {
		return []string{"video"}, nil
	})
	defer interactive.SetMultiSelectOverride(nil)

	interactive.SetConfirmOverride(func(message string, defaultVal bool) (bool, error) {
		return true, nil
	})
	defer interactive.SetConfirmOverride(nil)

	// Run cmdCombosAdd without flags
	if err := cmdCombosAdd([]string{}); err != nil {
		t.Fatalf("cmdCombosAdd wizard execution failed: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	resTopo, err := config.ParseTopology(data)
	if err != nil {
		t.Fatalf("parse topo: %v", err)
	}
	if len(resTopo.Combos) != 1 {
		t.Fatalf("expected 1 combo, got %d", len(resTopo.Combos))
	}
	cb := resTopo.Combos[0]
	if cb.Name != "wizard-combo" || cb.Mode != "fused" || len(cb.Capabilities) != 1 || cb.Capabilities[0] != "video" {
		t.Errorf("unexpected saved combo: %+v", cb)
	}
}

func TestCmdCombosAdd_AccountPinnedMembers(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(dotDir, "config.yaml")

	topo := config.Topology{
		Providers: map[string]config.Provider{
			"glm": {
				Dialect: "openai",
				BaseURL: "https://api.zhipu.ai",
				Models:  []string{"glm-4.7"},
				Accounts: []config.Account{
					{Name: "work"},
					{Name: "personal"},
				},
			},
		},
	}
	if err := config.WriteTopology(configPath, topo); err != nil {
		t.Fatalf("write topo: %v", err)
	}

	// 1. Fully-typed with valid pinned and unpinned members
	err := cmdCombosAdd([]string{"pinned-combo", "--members=glm@work:glm-4.7,glm:glm-4.7", "--mode=ordered"})
	if err != nil {
		t.Fatalf("expected success with pinned members, got: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	resTopo, err := config.ParseTopology(data)
	if err != nil {
		t.Fatalf("parse topo: %v", err)
	}
	if len(resTopo.Combos) != 1 {
		t.Fatalf("expected 1 combo, got %d", len(resTopo.Combos))
	}
	if resTopo.Combos[0].Members[0] != "glm@work:glm-4.7" || resTopo.Combos[0].Members[1] != "glm:glm-4.7" {
		t.Errorf("expected verbatim pinned and unpinned members, got: %v", resTopo.Combos[0].Members)
	}

	// 2. Fully-typed with unknown account fails at write validation
	err = cmdCombosAdd([]string{"bad-pinned", "--members=glm@ghost:glm-4.7,glm:glm-4.7"})
	if err == nil {
		t.Fatalf("expected error for unknown account pin, got nil")
	}
	if !strings.Contains(err.Error(), "references unknown account \"ghost\" for provider \"glm\"") {
		t.Errorf("expected unknown account error copy, got: %v", err)
	}
}

func TestRunComboAddWizard_AccountPinnedPicks(t *testing.T) {
	topo := config.Topology{
		Providers: map[string]config.Provider{
			"glm": {
				Dialect: "openai",
				BaseURL: "https://api.zhipu.ai",
				Models:  []string{"glm-4.7"},
				Accounts: []config.Account{
					{Name: "work"},
					{Name: "personal"},
				},
			},
		},
	}

	interactive.SetInputOverride(func(message, defaultVal string, validator func(string) error) (string, error) {
		return "pinned-wizard", nil
	})
	defer interactive.SetInputOverride(nil)

	selectCall := 0
	var firstOptions []string
	var secondOptions []string
	interactive.SetSelectOverride(func(message string, options []string) (string, error) {
		selectCall++
		if selectCall == 1 {
			firstOptions = options
			// Pick unpinned form
			return "glm:glm-4.7", nil
		}
		if selectCall == 2 {
			secondOptions = options
			// Pick pinned form
			return "glm@work:glm-4.7", nil
		}
		if selectCall == 3 {
			return "Done — enough members", nil
		}
		if selectCall == 4 {
			return "ordered - try members in sequence until one succeeds (default)", nil
		}
		return options[0], nil
	})
	defer interactive.SetSelectOverride(nil)

	interactive.SetMultiSelectOverride(func(message string, options []string) ([]string, error) {
		return nil, nil
	})
	defer interactive.SetMultiSelectOverride(nil)

	interactive.SetConfirmOverride(func(message string, defaultVal bool) (bool, error) {
		return true, nil
	})
	defer interactive.SetConfirmOverride(nil)

	name, members, mode, _, err := RunComboAddWizard(topo, "")
	if err != nil {
		t.Fatalf("unexpected wizard error: %v", err)
	}
	if name != "pinned-wizard" || mode != "ordered" {
		t.Errorf("unexpected name or mode: %s, %s", name, mode)
	}
	if len(members) != 2 || members[0] != "glm:glm-4.7" || members[1] != "glm@work:glm-4.7" {
		t.Errorf("unexpected members: %v", members)
	}

	// Verify first prompt offered both pinned and unpinned candidates
	foundPinnedInFirst := false
	foundUnpinnedInFirst := false
	for _, opt := range firstOptions {
		if opt == "glm@work:glm-4.7" {
			foundPinnedInFirst = true
		}
		if opt == "glm:glm-4.7" {
			foundUnpinnedInFirst = true
		}
	}
	if !foundPinnedInFirst || !foundUnpinnedInFirst {
		t.Errorf("expected first prompt to offer pinned and unpinned candidates: %v", firstOptions)
	}

	// Verify unpinned selection did not exclude pinned selection in second prompt
	foundPinnedInSecond := false
	for _, opt := range secondOptions {
		if opt == "glm@work:glm-4.7" {
			foundPinnedInSecond = true
			break
		}
	}
	if !foundPinnedInSecond {
		t.Errorf("expected glm@work:glm-4.7 to still be offered in second prompt: %v", secondOptions)
	}
}
