package agent

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type codexAdapter struct{}

func init() {
	Register(&codexAdapter{})
}

func (c *codexAdapter) ID() string       { return "codex" }
func (c *codexAdapter) Name() string     { return "Codex CLI" }
func (c *codexAdapter) Dialect() string  { return "openai-responses" }
func (c *codexAdapter) NeedsModel() bool { return true }

func (c *codexAdapter) ModelSlots() []ModelSlot {
	return []ModelSlot{
		{ID: "model", Name: "Primary Model", Kind: SlotSingle, Required: true},
		{ID: "subagent", Name: "Subagent Model", Kind: SlotSingle, Required: false},
	}
}

func (c *codexAdapter) getConfigPath() string {
	return expandHome("~/.codex/config.toml")
}

func (c *codexAdapter) getAuthPath() string {
	return expandHome("~/.codex/auth.json")
}

func (c *codexAdapter) Detect() (Status, error) {
	configPath := c.getConfigPath()
	installed := false

	cmdName := "which"
	if runtime.GOOS == "windows" {
		cmdName = "where"
	}
	if err := exec.Command(cmdName, "codex").Run(); err == nil {
		installed = true
	} else if _, err := os.Stat(configPath); err == nil {
		installed = true
	}

	pointed := false
	if installed {
		m, err := readTOMLMap(configPath)
		if err == nil {
			if mp, ok := m["model_provider"].(string); ok && mp == "tinyroute" {
				pointed = true
			} else if sec, ok := m["model_providers"].(map[string]any); ok {
				if _, ok := sec["tinyroute"]; ok {
					pointed = true
				}
			}
		}
	}

	return Status{
		Installed:          installed,
		PointedAtTinyRoute: pointed,
		ConfigPath:         configPath,
	}, nil
}

func (c *codexAdapter) Apply(input ApplyInput) (Result, error) {
	configPath := c.getConfigPath()
	authPath := c.getAuthPath()

	bakConfig, err := backup(configPath)
	if err != nil {
		return Result{}, err
	}
	bakAuth, _ := backup(authPath)

	baseURL := input.BaseURL
	if !strings.HasSuffix(baseURL, "/v1") && !strings.HasSuffix(baseURL, "/v1/") {
		baseURL = strings.TrimRight(baseURL, "/") + "/v1"
	}

	// 1. Update config.toml
	rootFields := map[string]any{
		"model_provider": "tinyroute",
	}
	if input.Model != "" {
		rootFields["model"] = input.Model
	}

	providerFields := map[string]any{
		"name":     "tinyroute",
		"base_url": baseURL,
		"wire_api": "responses",
	}

	if err := updateTOMLProvider(configPath, "model_providers", "tinyroute", providerFields, rootFields); err != nil {
		return Result{}, err
	}

	subagentModel := input.Model
	if input.ModelSlots != nil && input.ModelSlots["subagent"] != "" {
		subagentModel = input.ModelSlots["subagent"]
	}
	if subagentModel != "" {
		subagentFields := map[string]any{
			"model": subagentModel,
		}
		_ = updateTOMLProvider(configPath, "agents", "subagent", subagentFields, nil)
	}

	// 2. Update auth.json
	setAuth := map[string]any{
		"OPENAI_API_KEY": input.APIKey,
		"auth_mode":      "apikey",
	}
	if err := updateJSONEnv(authPath, "", nil, setAuth); err != nil {
		return Result{}, err
	}

	bak := bakConfig
	if bak == "" {
		bak = bakAuth
	}

	return Result{
		Files:  []string{configPath, authPath},
		Key:    input.APIKey,
		Backup: bak,
	}, nil
}

func (c *codexAdapter) Reset() error {
	configPath := c.getConfigPath()
	authPath := c.getAuthPath()

	// Check existing config to see if model_provider is tinyroute
	m, err := readTOMLMap(configPath)
	var removeRoots []string
	if err == nil {
		if mp, ok := m["model_provider"].(string); ok && mp == "tinyroute" {
			removeRoots = append(removeRoots, "model", "model_provider")
		}
	}

	_ = resetTOMLProvider(configPath, "model_providers", "tinyroute", removeRoots)
	_ = resetTOMLProvider(configPath, "agents", "subagent", nil)

	_ = resetJSONEnv(authPath, "", nil, []string{"OPENAI_API_KEY", "auth_mode"})

	return nil
}
