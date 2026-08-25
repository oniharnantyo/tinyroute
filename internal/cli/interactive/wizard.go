package interactive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oniharnantyo/tinyroute/internal/auth"
	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/preset"
	"github.com/pterm/pterm"
)

// WizardState holds user selections throughout the setup wizard.
type WizardState struct {
	SelectedProviders []preset.Preset
	Credentials       map[string]string // provider.Name -> API Key
	KeyName           string
	GeneratedPlainKey string
	GeneratedKeyID    string
}

// RunInitWizard executes the multi-step guided setup wizard for tinyroute.
func RunInitWizard() error {
	if !CanPrompt() {
		pterm.Info.Println("Non-interactive terminal detected. Running minimal initialization.")
		return nil
	}

	pterm.DefaultHeader.WithFullWidth().WithBackgroundStyle(pterm.NewStyle(pterm.BgDarkGray)).Println("tinyroute Guided Setup Wizard")
	pterm.Println("This wizard will help you configure tinyroute, set up providers, and create your first API key.")
	pterm.Println()

	confirm, err := Confirm("Would you like to proceed with interactive setup?", true)
	if err != nil || !confirm {
		pterm.Info.Println("Setup cancelled.")
		return nil
	}

	state := &WizardState{
		Credentials: make(map[string]string),
		KeyName:     "default",
	}

	// Step 1: Select Providers
	presets := preset.All()
	var providerNames []string
	for _, p := range presets {
		providerNames = append(providerNames, fmt.Sprintf("%s (%s)", p.Name, p.CredentialVar))
	}

	pterm.DefaultSection.Println("Step 1: Select LLM Provider Presets")
	selectedPresetName, err := Select("Choose a provider to configure:", providerNames)
	if err == nil && selectedPresetName != "" {
		parts := strings.SplitN(selectedPresetName, " (", 2)
		if p := preset.Get(parts[0]); p != nil {
			state.SelectedProviders = append(state.SelectedProviders, *p)
		}
	}

	// Step 2: Credentials
	if len(state.SelectedProviders) > 0 {
		pterm.DefaultSection.Println("Step 2: Collect Provider Credentials")
		for _, p := range state.SelectedProviders {
			apiKey, err := Password(fmt.Sprintf("Enter API key for %s (or leave empty to set via env %s):", p.Name, p.CredentialVar))
			if err == nil && apiKey != "" {
				state.Credentials[p.Name] = apiKey
			}
		}
	}

	// Step 3: API Key Creation
	pterm.DefaultSection.Println("Step 3: Initial API Key")
	keyName, err := Input("Enter name for initial client API key:", "default", func(val string) error {
		if strings.TrimSpace(val) == "" {
			return fmt.Errorf("key name cannot be empty")
		}
		return nil
	})
	if err == nil && keyName != "" {
		state.KeyName = keyName
	}

	// Step 4: Summary & Confirmation
	pterm.DefaultSection.Println("Summary of Configuration")
	pterm.Println("Providers to add:", len(state.SelectedProviders))
	for _, p := range state.SelectedProviders {
		status := "configured"
		if state.Credentials[p.Name] == "" {
			status = fmt.Sprintf("environment variable (%s)", p.CredentialVar)
		}
		pterm.Printf("  • %s (%s): %s\n", p.Name, p.Dialect, status)
	}
	pterm.Printf("API Key name: %s\n", state.KeyName)
	pterm.Println()

	saveConfirm, err := Confirm("Save configuration and generate API key?", true)
	if err != nil || !saveConfirm {
		pterm.Info.Println("Setup cancelled; no changes saved.")
		return nil
	}

	// Save configuration
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	dir := filepath.Join(home, ".tinyroute")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	// Write .env
	envPath := filepath.Join(dir, ".env")
	envContent := "# tinyroute environment\n"
	for provName, apiKey := range state.Credentials {
		p := preset.Get(provName)
		if p != nil && p.CredentialVar != "" {
			envContent += fmt.Sprintf("%s=%s\n", p.CredentialVar, apiKey)
		}
	}
	if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
		return fmt.Errorf("write %s: %w", envPath, err)
	}

	// Write config.yaml
	configPath := filepath.Join(dir, "config.yaml")
	topo := config.Topology{
		Providers: make(map[string]config.Provider),
	}
	for _, p := range state.SelectedProviders {
		apiKey := ""
		if k, ok := state.Credentials[p.Name]; ok && k != "" {
			apiKey = k
		} else if envVal := os.Getenv(p.CredentialVar); envVal != "" {
			apiKey = envVal
		}
		topo.Providers[p.Name] = config.Provider{
			Dialect:   p.Dialect,
			BaseURL:   p.BaseURL,
			Transport: p.Transport,
			APIKey:    apiKey,
		}
	}
	if err := config.WriteTopology(configPath, topo); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}

	// Generate key
	keysPath := filepath.Join(dir, "keys.json")
	plainKey, keyObj, err := auth.GenerateKey(state.KeyName)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	kf := auth.KeyFile{Keys: []auth.Key{keyObj}}
	if err := auth.WriteKeyFile(keysPath, kf); err != nil {
		return fmt.Errorf("write %s: %w", keysPath, err)
	}

	state.GeneratedPlainKey = plainKey
	state.GeneratedKeyID = keyObj.ID

	pterm.Success.Println("Initialization complete!")
	pterm.Println()
	pterm.Println("Your API key (shown once, store it now):")
	pterm.Println("  " + plainKey)
	pterm.Println()
	pterm.Println("Start tinyroute with: tinyroute serve")

	return nil
}
