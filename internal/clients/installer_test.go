package clients_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/clients"
)

func TestInstaller_PlanAndApply(t *testing.T) {
	tempDir := t.TempDir()
	keysPath := filepath.Join(tempDir, "keys.json")

	// Set HOME so client configs go into tempDir
	t.Setenv("HOME", tempDir)

	installer := clients.NewInstaller("127.0.0.1:8080", keysPath)

	t.Run("Plan unknown client", func(t *testing.T) {
		_, err := installer.Plan(clients.InstallRequest{
			ClientID: "nonexistent",
		})
		if err == nil {
			t.Fatalf("expected error for unknown client")
		}
	})

	t.Run("Plan without writing", func(t *testing.T) {
		plan, err := installer.Plan(clients.InstallRequest{
			ClientID: "claude",
		})
		if err != nil {
			t.Fatalf("unexpected error planning claude: %v", err)
		}
		if plan.ClientID != "claude" {
			t.Errorf("expected client id claude, got %s", plan.ClientID)
		}
		if plan.Dialect != "anthropic" {
			t.Errorf("expected dialect anthropic, got %s", plan.Dialect)
		}
		if plan.BaseURL != "http://127.0.0.1:8080/anthropic" {
			t.Errorf("expected http://127.0.0.1:8080/anthropic, got %s", plan.BaseURL)
		}
		if plan.KeyStrategy != clients.KeyStrategyMint {
			t.Errorf("expected KeyStrategyMint, got %s", plan.KeyStrategy)
		}

		// Ensure Plan made no file changes
		if _, err := os.Stat(plan.ConfigPath); err == nil {
			t.Errorf("Plan should not write any files, but config file exists at %s", plan.ConfigPath)
		}
	})

	t.Run("Apply with Mint", func(t *testing.T) {
		plan, err := installer.Plan(clients.InstallRequest{
			ClientID:    "claude",
			KeyStrategy: clients.KeyStrategyMint,
			KeyName:     "custom-claude-key",
		})
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}

		res, err := installer.Apply(plan)
		if err != nil {
			t.Fatalf("Apply failed: %v", err)
		}

		if res.Key == "" {
			t.Errorf("expected minted key in result")
		}
		if len(res.Files) == 0 {
			t.Errorf("expected written files in result")
		}

		// Check keys.json was created
		if _, err := os.Stat(keysPath); err != nil {
			t.Errorf("expected keys.json at %s: %v", keysPath, err)
		}
	})

	t.Run("Apply with Reuse Key", func(t *testing.T) {
		callerKey := "tr_live_caller_provided_token"
		plan, err := installer.Plan(clients.InstallRequest{
			ClientID:    "claude",
			APIKey:      callerKey,
			KeyStrategy: clients.KeyStrategyReuse,
		})
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}

		res, err := installer.Apply(plan)
		if err != nil {
			t.Fatalf("Apply failed: %v", err)
		}

		if res.Key != callerKey {
			t.Errorf("expected reusable key %s, got %s", callerKey, res.Key)
		}
	})
}

func TestDialectBaseURL(t *testing.T) {
	tests := []struct {
		listen  string
		dialect string
		want    string
	}{
		{":8080", "anthropic", "http://127.0.0.1:8080/anthropic"},
		{"0.0.0.0:9090", "openai", "http://127.0.0.1:9090/openai/v1"},
		{"127.0.0.1:3000", "gemini", "http://127.0.0.1:3000/gemini"},
		{"https://gateway.example.com", "anthropic", "https://gateway.example.com/anthropic"},
	}

	for _, tt := range tests {
		got := clients.DialectBaseURL(tt.listen, tt.dialect)
		if got != tt.want {
			t.Errorf("DialectBaseURL(%q, %q) = %q, want %q", tt.listen, tt.dialect, got, tt.want)
		}
	}
}
