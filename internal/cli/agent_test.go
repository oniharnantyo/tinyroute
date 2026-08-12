package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/cli/interactive"
)

func TestCLIAgentCommands(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	falseVal := false
	interactive.SetCanPromptOverride(&falseVal)
	defer interactive.SetCanPromptOverride(nil)

	// 1. Install missing arg in non-interactive mode -> error
	err := cmdAgentInstall([]string{"--no-interactive"})
	if err == nil {
		t.Fatalf("expected error when agent id omitted in non-interactive mode")
	}

	// 2. Install claude with explicit flags
	err = cmdAgentInstall([]string{"claude", "--no-interactive", "--api-key=tr_live_test123", "--base-url=http://127.0.0.1:8080/anthropic"})
	if err != nil {
		t.Fatalf("cmdAgentInstall claude failed: %v", err)
	}

	// Verify ~/.claude/settings.json exists
	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings.json missing after install: %v", err)
	}

	// 3. Status command
	err = cmdAgentStatus([]string{"claude", "--no-interactive"})
	if err != nil {
		t.Fatalf("cmdAgentStatus failed: %v", err)
	}

	// 4. Uninstall claude with --force
	err = cmdAgentUninstall([]string{"claude", "--force"})
	if err != nil {
		t.Fatalf("cmdAgentUninstall failed: %v", err)
	}

	// Verify settings.json after uninstall
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	if strings.Contains(string(data), "ANTHROPIC_BASE_URL") {
		t.Errorf("expected ANTHROPIC_BASE_URL to be removed after uninstall")
	}
}

func TestDialectBaseURL(t *testing.T) {
	tests := []struct {
		listen  string
		dialect string
		want    string
	}{
		{":8080", "anthropic", "http://127.0.0.1:8080/anthropic"},
		{"0.0.0.0:8080", "openairesponses", "http://127.0.0.1:8080/openai/v1"},
		{"127.0.0.1:8080", "anthropic", "http://127.0.0.1:8080/anthropic"},
		{"http://localhost:8080", "anthropic", "http://localhost:8080/anthropic"},
	}

	for _, tt := range tests {
		got := dialectBaseURL(tt.listen, tt.dialect)
		if got != tt.want {
			t.Errorf("dialectBaseURL(%q, %q) = %q, want %q", tt.listen, tt.dialect, got, tt.want)
		}
	}
}

func TestAgentInstallCustomNameAndReuse(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	_ = os.MkdirAll(dotDir, 0700)

	falseVal := false
	interactive.SetCanPromptOverride(&falseVal)
	defer interactive.SetCanPromptOverride(nil)

	// Install with custom key name flag
	err := cmdAgentInstall([]string{"codex", "--no-interactive", "--name=custom-codex-key"})
	if err != nil {
		t.Fatalf("cmdAgentInstall with --name failed: %v", err)
	}

	keysPath := filepath.Join(dotDir, "keys.json")
	data, err := os.ReadFile(keysPath)
	if err != nil {
		t.Fatalf("read keys.json: %v", err)
	}
	if !strings.Contains(string(data), "custom-codex-key") {
		t.Errorf("expected custom-codex-key in keys.json, got: %s", string(data))
	}
	if !strings.Contains(string(data), "openai-responses:*") {
		t.Errorf("expected openai-responses:* permission scope in keys.json, got: %s", string(data))
	}
}

func TestCmdAgentBuilder(t *testing.T) {
	cmd := cmdAgent()
	if cmd == nil || cmd.Name != "agent" {
		t.Fatalf("expected cmdAgent to return agent command")
	}
	if len(cmd.Commands) != 3 {
		t.Fatalf("expected 3 subcommands for agent, got %d", len(cmd.Commands))
	}
}

func TestTruncateKeyDisplay(t *testing.T) {
	if got := truncateKeyDisplay("short"); got != "short" {
		t.Errorf("truncateKeyDisplay(short) = %q, want short", got)
	}
	if got := truncateKeyDisplay("1234567890"); got != "1234...7890" {
		t.Errorf("truncateKeyDisplay(long) = %q, want 1234...7890", got)
	}
}
