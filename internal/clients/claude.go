package clients

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type claudeAdapter struct{}

func init() {
	Register(&claudeAdapter{})
}

func (c *claudeAdapter) ID() string       { return "claude" }
func (c *claudeAdapter) Name() string     { return "Claude Code" }
func (c *claudeAdapter) Dialect() string  { return "anthropic" }
func (c *claudeAdapter) NeedsModel() bool { return true }

func (c *claudeAdapter) ModelSlots() []ModelSlot {
	return []ModelSlot{
		{ID: "opus", Name: "Opus Default Model", Kind: SlotSingle, Required: false},
		{ID: "sonnet", Name: "Sonnet Default Model", Kind: SlotSingle, Required: false},
		{ID: "haiku", Name: "Haiku Default Model", Kind: SlotSingle, Required: false},
		{ID: "fable", Name: "Fable Default Model", Kind: SlotSingle, Required: false},
		{ID: "subagent", Name: "Subagent Model", Kind: SlotSingle, Required: false},
	}
}

func (c *claudeAdapter) getSettingsPath() string {
	return expandHome("~/.claude/settings.json")
}

func (c *claudeAdapter) Detect() (Status, error) {
	path := c.getSettingsPath()
	installed := false

	// Check CLI executable presence
	cmdName := "which"
	if runtime.GOOS == "windows" {
		cmdName = "where"
	}
	if err := exec.Command(cmdName, "claude").Run(); err == nil {
		installed = true
	} else if _, err := os.Stat(path); err == nil {
		installed = true
	}

	pointed := false
	currentBaseURL := ""
	maskedKey := ""
	rawKey := ""
	slotValues := make(map[string]string)
	if installed {
		m, err := readJSONMap(path)
		if err == nil {
			if envMap, ok := m["env"].(map[string]any); ok {
				if baseURL, ok := envMap["ANTHROPIC_BASE_URL"].(string); ok && baseURL != "" {
					pointed = true
					currentBaseURL = baseURL
				}
				if token, ok := envMap["ANTHROPIC_AUTH_TOKEN"].(string); ok && token != "" {
					rawKey = token
					maskedKey = MaskKey(token)
				}
				for _, slot := range c.ModelSlots() {
					envKey := "ANTHROPIC_DEFAULT_" + strings.ToUpper(slot.ID) + "_MODEL"
					if slot.ID == "subagent" {
						envKey = "CLAUDE_CODE_SUBAGENT_MODEL"
					}
					if val, ok := envMap[envKey].(string); ok && val != "" {
						slotValues[slot.ID] = val
					} else if val, ok := envMap["ANTHROPIC_DEFAULT_MODEL_"+strings.ToUpper(slot.ID)].(string); ok && val != "" {
						slotValues[slot.ID] = val
					}
				}
			}
		}
	}

	return Status{
		Installed:          installed,
		PointedAtTinyRoute: pointed,
		ConfigPath:         path,
		CurrentBaseURL:     currentBaseURL,
		MaskedKey:          maskedKey,
		RawKey:             rawKey,
		SlotValues:         slotValues,
	}, nil
}

func (c *claudeAdapter) Apply(input ApplyInput) (Result, error) {
	path := c.getSettingsPath()

	bak, err := backup(path)
	if err != nil {
		return Result{}, err
	}

	apiKey := input.APIKey
	if apiKey == "" {
		if m, err := readJSONMap(path); err == nil {
			if envMap, ok := m["env"].(map[string]any); ok {
				if cur, ok := envMap["ANTHROPIC_AUTH_TOKEN"].(string); ok && cur != "" {
					apiKey = cur
				}
			}
		}
	}

	baseURL := strings.TrimRight(input.BaseURL, "/")

	setFields := map[string]any{
		"ANTHROPIC_BASE_URL":   baseURL,
		"ANTHROPIC_AUTH_TOKEN": apiKey,
	}

	if input.ContextWindow != "" {
		setFields["CLAUDE_CODE_MAX_CONTEXT_TOKENS"] = input.ContextWindow
	}

	if input.ModelSlots != nil {
		if v := input.ModelSlots["opus"]; v != "" {
			setFields["ANTHROPIC_DEFAULT_OPUS_MODEL"] = v
		}
		if v := input.ModelSlots["sonnet"]; v != "" {
			setFields["ANTHROPIC_DEFAULT_SONNET_MODEL"] = v
		}
		if v := input.ModelSlots["haiku"]; v != "" {
			setFields["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = v
		}
		if v := input.ModelSlots["fable"]; v != "" {
			setFields["ANTHROPIC_DEFAULT_FABLE_MODEL"] = v
		}
		if v := input.ModelSlots["subagent"]; v != "" {
			setFields["CLAUDE_CODE_SUBAGENT_MODEL"] = v
		}
	}

	rootFields := map[string]any{
		"hasCompletedOnboarding": true,
	}

	if err := updateJSONEnv(path, "env", setFields, rootFields); err != nil {
		return Result{}, err
	}

	return Result{
		Files:  []string{path},
		Key:    apiKey,
		Backup: bak,
	}, nil
}

func (c *claudeAdapter) Reset() error {
	path := c.getSettingsPath()
	removeEnvKeys := []string{
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"API_TIMEOUT_MS",
		"CLAUDE_CODE_MAX_CONTEXT_TOKENS",
	}
	return resetJSONEnv(path, "env", removeEnvKeys, nil)
}
