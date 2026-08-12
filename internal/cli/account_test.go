package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/credential"
)

func setupAccountTestHome(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	tinyDir := filepath.Join(homeDir, ".tinyroute")
	if err := os.MkdirAll(tinyDir, 0700); err != nil {
		t.Fatalf("failed to create test home dir: %v", err)
	}
	t.Setenv("HOME", homeDir)
	t.Setenv("TINYROUTE_CONFIG", filepath.Join(tinyDir, "config.yaml"))
	t.Setenv("TINYROUTE_CREDENTIALS", filepath.Join(tinyDir, "credentials.json"))
	t.Setenv("TINYROUTE_STATE", filepath.Join(tinyDir, "state.json"))

	topo := config.Topology{
		Providers: map[string]config.Provider{
			"openai": {
				Dialect: "openai",
				BaseURL: "https://api.openai.com/v1",
				APIKey:  "sk-legacy-key",
				Accounts: []config.Account{
					{
						Name:   "primary",
						Type:   "static",
						APIKey: "sk-proj-primary123456",
					},
					{
						Name:   "secondary",
						Type:   "static",
						APIKey: "sk-proj-secondary654321",
					},
				},
				Selection: config.StrategyRoundRobin,
			},
			"anthropic": {
				Dialect:  "anthropic",
				BaseURL:  "https://api.anthropic.com/v1",
				Accounts: []config.Account{},
			},
		},
	}
	if err := config.WriteTopology(filepath.Join(tinyDir, "config.yaml"), topo); err != nil {
		t.Fatalf("failed to write topology: %v", err)
	}

	credStore, err := credential.NewStore(filepath.Join(tinyDir, "credentials.json"))
	if err != nil {
		t.Fatalf("failed to create cred store: %v", err)
	}
	_ = credStore.Save(credential.OAuthRecord{
		Provider:     "anthropic",
		Account:      "work",
		RefreshToken: "rt_work_secret_token",
	})

	return homeDir
}

func TestAccountAddNonInteractive(t *testing.T) {
	setupAccountTestHome(t)

	// Add static account
	cmd := cmdProvidersAccount()
	args := []string{"account", "add", "openai", "backup", "--type=static", "--key=sk-proj-backup999", "--no-interactive"}
	if err := cmd.Run(t.Context(), args); err != nil {
		t.Fatalf("account add failed: %v", err)
	}

	svc, err := config.LoadService()
	if err != nil {
		t.Fatalf("load service: %v", err)
	}
	data, _ := os.ReadFile(svc.ConfigPath)
	topo, _ := config.ParseRawTopology(data)

	p := topo.Providers["openai"]
	found := false
	for _, acc := range p.Accounts {
		if acc.Name == "backup" {
			found = true
			if acc.APIKey != "sk-proj-backup999" {
				t.Errorf("expected API key sk-proj-backup999, got %q", acc.APIKey)
			}
		}
	}
	if !found {
		t.Error("account 'backup' was not added to provider 'openai'")
	}
}

func TestAccountAddDuplicateRejection(t *testing.T) {
	setupAccountTestHome(t)

	cmd := cmdProvidersAccount()
	args := []string{"account", "add", "openai", "primary", "--type=static", "--key=sk-new", "--no-interactive"}
	err := cmd.Run(t.Context(), args)
	if err == nil {
		t.Fatal("expected error on duplicate account name, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got %v", err)
	}
}

func TestAccountListMasked(t *testing.T) {
	setupAccountTestHome(t)

	err := cmdAccountList([]string{"openai"}, false)
	if err != nil {
		t.Fatalf("account list failed: %v", err)
	}

	// Verify plaintext secrets are never printed
	var stdout bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	_ = cmdAccountList([]string{"openai"}, false)

	_ = w.Close()
	os.Stdout = oldStdout
	_, _ = stdout.ReadFrom(r)
	outStr := stdout.String()

	if strings.Contains(outStr, "sk-proj-primary123456") {
		t.Error("account list leaked plaintext credential for primary account!")
	}
	if !strings.Contains(outStr, "primary") || !strings.Contains(outStr, "secondary") {
		t.Errorf("account list output missing account names: %s", outStr)
	}
}

func TestAccountRemove(t *testing.T) {
	setupAccountTestHome(t)

	err := cmdAccountRemove([]string{"openai", "secondary"}, false)
	if err != nil {
		t.Fatalf("account remove failed: %v", err)
	}

	svc, _ := config.LoadService()
	data, _ := os.ReadFile(svc.ConfigPath)
	topo, _ := config.ParseRawTopology(data)

	p := topo.Providers["openai"]
	for _, acc := range p.Accounts {
		if acc.Name == "secondary" {
			t.Error("account 'secondary' was not removed")
		}
	}
}

func TestAccountSelectStrategy(t *testing.T) {
	setupAccountTestHome(t)

	err := cmdAccountSelect([]string{"openai"}, "sticky_round_robin", false)
	if err != nil {
		t.Fatalf("account select strategy failed: %v", err)
	}

	svc, _ := config.LoadService()
	data, _ := os.ReadFile(svc.ConfigPath)
	topo, _ := config.ParseRawTopology(data)

	if topo.Providers["openai"].Selection != config.StrategyStickyRoundRobin {
		t.Errorf("expected strategy %s, got %s", config.StrategyStickyRoundRobin, topo.Providers["openai"].Selection)
	}
}

func TestAccountImportLineFormat(t *testing.T) {
	setupAccountTestHome(t)

	tmpFile := filepath.Join(t.TempDir(), "accounts.txt")
	content := "acc1|sk-key1\nacc2|sk-key2\n"
	_ = os.WriteFile(tmpFile, []byte(content), 0600)

	err := cmdAccountImport([]string{"anthropic"}, tmpFile, false)
	if err != nil {
		t.Fatalf("account import failed: %v", err)
	}

	svc, _ := config.LoadService()
	data, _ := os.ReadFile(svc.ConfigPath)
	topo, _ := config.ParseRawTopology(data)

	p := topo.Providers["anthropic"]
	if len(p.Accounts) != 2 {
		t.Fatalf("expected 2 imported accounts, got %d", len(p.Accounts))
	}
	if p.Accounts[0].Name != "acc1" || p.Accounts[0].APIKey != "sk-key1" {
		t.Errorf("imported account 0 mismatch: %+v", p.Accounts[0])
	}
}

func TestAuthSetAccountFlag(t *testing.T) {
	setupAccountTestHome(t)

	args := []string{"openai", "--account=prod", "--no-interactive"}
	r, w, _ := os.Pipe()
	_, _ = w.WriteString("sk-proj-prod-key\n")
	_ = w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	err := cmdAuthSet(args)
	if err != nil {
		t.Fatalf("cmdAuthSet with --account failed: %v", err)
	}

	svc, _ := config.LoadService()
	data, _ := os.ReadFile(svc.ConfigPath)
	topo, _ := config.ParseRawTopology(data)

	p := topo.Providers["openai"]
	found := false
	for _, acc := range p.Accounts {
		if acc.Name == "prod" && acc.APIKey == "sk-proj-prod-key" {
			found = true
			break
		}
	}
	if !found {
		t.Error("cmdAuthSet with --account did not set provider account credential")
	}
}
