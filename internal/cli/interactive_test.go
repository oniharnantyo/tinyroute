package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/auth"
	"github.com/oniharnantyo/tinyroute/internal/cli/interactive"
	"github.com/oniharnantyo/tinyroute/internal/config"
)

func setupTestHome(t *testing.T) string {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	return tmpDir
}

func TestInteractiveKeysRevoke(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	keysPath := filepath.Join(dotDir, "keys.json")
	_, key, err := auth.GenerateKey("test-key")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	kf := auth.KeyFile{Keys: []auth.Key{key}}
	if err := auth.WriteKeyFile(keysPath, kf); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	// Default interactive mode: when confirm returns default (false), revocation is cancelled
	falseVal := false
	interactive.SetCanPromptOverride(&falseVal)
	defer interactive.SetCanPromptOverride(nil)

	// Call without any flags (default is interactive)
	err = cmdKeysRevoke([]string{key.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify key was NOT revoked because confirmation defaulted to false
	updatedKf, err := loadKeyFile(keysPath)
	if err != nil {
		t.Fatalf("load key file: %v", err)
	}
	if updatedKf.Keys[0].Disabled {
		t.Errorf("expected key to remain enabled when non-interactive confirm defaults to false")
	}

	// With --force, key should be revoked directly without confirmation
	err = cmdKeysRevoke([]string{key.ID, "--force"})
	if err != nil {
		t.Fatalf("unexpected error with --force: %v", err)
	}
	updatedKf, err = loadKeyFile(keysPath)
	if err != nil {
		t.Fatalf("load key file: %v", err)
	}
	if !updatedKf.Keys[0].Disabled {
		t.Errorf("expected key to be disabled with --force")
	}
}

func TestNoInteractiveFlagRevokesDirectly(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	keysPath := filepath.Join(dotDir, "keys.json")
	_, key, err := auth.GenerateKey("test-key-2")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	kf := auth.KeyFile{Keys: []auth.Key{key}}
	if err := auth.WriteKeyFile(keysPath, kf); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	// With --no-interactive, key should be revoked directly without confirmation prompt
	err = cmdKeysRevoke([]string{key.ID, "--no-interactive"})
	if err != nil {
		t.Fatalf("cmdKeysRevoke error: %v", err)
	}

	updatedKf, err := loadKeyFile(keysPath)
	if err != nil {
		t.Fatalf("load key file: %v", err)
	}
	if !updatedKf.Keys[0].Disabled {
		t.Errorf("expected key to be disabled with --no-interactive")
	}
}

func TestInteractiveKeysCreate(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	falseVal := false
	interactive.SetCanPromptOverride(&falseVal)
	defer interactive.SetCanPromptOverride(nil)

	err := cmdKeysCreate([]string{"--name=my-test-key"})
	if err != nil {
		t.Fatalf("cmdKeysCreate error: %v", err)
	}

	keysPath := filepath.Join(dotDir, "keys.json")
	kf, err := loadKeyFile(keysPath)
	if err != nil {
		t.Fatalf("loadKeyFile error: %v", err)
	}
	if len(kf.Keys) != 1 || kf.Keys[0].Name != "my-test-key" {
		t.Errorf("expected key name 'my-test-key', got %+v", kf.Keys)
	}
}

func TestInteractiveProviderAdd(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key")

	falseVal := false
	interactive.SetCanPromptOverride(&falseVal)
	defer interactive.SetCanPromptOverride(nil)

	// Default interactive select with no preset arg picks first preset (anthropic)
	err := cmdAdd([]string{})
	if err != nil {
		t.Fatalf("cmdAdd default interactive error: %v", err)
	}

	configPath := filepath.Join(dotDir, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config error: %v", err)
	}
	topo, err := config.ParseTopology(data)
	if err != nil {
		t.Fatalf("parse topo error: %v", err)
	}

	if _, ok := topo.Providers["anthropic"]; !ok {
		t.Errorf("expected provider 'anthropic' to be added via interactive fallback")
	}
}

func TestMultipleOpenAICompatibleProviders(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Add ollama
	if err := cmdAdd([]string{"openai-compatible", "ollama", "--base-url=http://localhost:11434/v1"}); err != nil {
		t.Fatalf("add ollama: %v", err)
	}

	// Add vllm
	if err := cmdAdd([]string{"openai-compatible", "vllm", "--base-url=http://localhost:8000/v1"}); err != nil {
		t.Fatalf("add vllm: %v", err)
	}

	configPath := filepath.Join(dotDir, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config error: %v", err)
	}
	topo, err := config.ParseRawTopology(data)
	if err != nil {
		t.Fatalf("parse topo error: %v", err)
	}

	ollama, ok1 := topo.Providers["ollama"]
	vllm, ok2 := topo.Providers["vllm"]

	if !ok1 || ollama.BaseURL != "http://localhost:11434/v1" {
		t.Errorf("ollama provider mismatch: %+v", ollama)
	}
	if !ok2 || vllm.BaseURL != "http://localhost:8000/v1" {
		t.Errorf("vllm provider mismatch: %+v", vllm)
	}
}

func TestInteractiveCompact(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	falseVal := false
	interactive.SetCanPromptOverride(&falseVal)
	defer interactive.SetCanPromptOverride(nil)

	err := cmdCompact([]string{})
	if err != nil {
		t.Fatalf("cmdCompact error: %v", err)
	}
}

func TestInteractiveInitWizard(t *testing.T) {
	setupTestHome(t)

	falseVal := false
	interactive.SetCanPromptOverride(&falseVal)
	defer interactive.SetCanPromptOverride(nil)

	err := cmdInit([]string{})
	if err != nil {
		t.Fatalf("cmdInit default interactive error: %v", err)
	}
}

func TestProviderModelCommands(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	falseVal := false
	interactive.SetCanPromptOverride(&falseVal)
	defer interactive.SetCanPromptOverride(nil)

	// 1. Setup provider
	t.Setenv("OPENAI_API_KEY", "sk-test-key")
	if err := cmdAdd([]string{"openai", "--no-interactive"}); err != nil {
		t.Fatalf("setup provider: %v", err)
	}

	// 2. Add models using --models flag
	if err := cmdProviderModelAdd([]string{"openai", "--models=gpt-4o,gpt-4o-mini", "--no-interactive"}); err != nil {
		t.Fatalf("provider model add: %v", err)
	}

	configPath := filepath.Join(dotDir, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config error: %v", err)
	}
	topo, err := config.ParseRawTopology(data)
	if err != nil {
		t.Fatalf("parse topo error: %v", err)
	}

	models := topo.Providers["openai"].Models
	if len(models) != 2 || models[0] != "gpt-4o" || models[1] != "gpt-4o-mini" {
		t.Errorf("expected whitelisted models [gpt-4o gpt-4o-mini], got %v", models)
	}

	// 3. List models
	if err := cmdProviderModelList([]string{"openai"}); err != nil {
		t.Fatalf("provider model list single provider error: %v", err)
	}
	if err := cmdProviderModelList([]string{}); err != nil {
		t.Fatalf("provider model list all error: %v", err)
	}

	// 4. Remove model explicitly
	if err := cmdProviderModelRemove([]string{"openai", "gpt-4o", "--no-interactive"}); err != nil {
		t.Fatalf("provider model remove explicit error: %v", err)
	}

	data, _ = os.ReadFile(configPath)
	topo, _ = config.ParseRawTopology(data)
	models = topo.Providers["openai"].Models
	if len(models) != 1 || models[0] != "gpt-4o-mini" {
		t.Errorf("expected remaining model [gpt-4o-mini], got %v", models)
	}

	// 5. Remove remaining model with single candidate auto-selection
	if err := cmdProviderModelRemove([]string{"--no-interactive"}); err != nil {
		t.Fatalf("provider model remove auto-select error: %v", err)
	}

	data, _ = os.ReadFile(configPath)
	topo, _ = config.ParseRawTopology(data)
	models = topo.Providers["openai"].Models
	if len(models) != 0 {
		t.Errorf("expected 0 remaining models, got %v", models)
	}

	// 6. Removing when no models exist should exit informatively without error
	if err := cmdProviderModelRemove([]string{"--no-interactive"}); err != nil {
		t.Fatalf("provider model remove empty exit error: %v", err)
	}
}
