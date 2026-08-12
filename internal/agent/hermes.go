package agent

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type hermesAdapter struct{}

func init() {
	Register(&hermesAdapter{})
}

func (h *hermesAdapter) ID() string       { return "hermes" }
func (h *hermesAdapter) Name() string     { return "Hermes Agent" }
func (h *hermesAdapter) Dialect() string  { return "openai" }
func (h *hermesAdapter) NeedsModel() bool { return true }

func (h *hermesAdapter) ModelSlots() []ModelSlot {
	return []ModelSlot{
		{ID: "model", Name: "Default Model", Kind: SlotSingle, Required: true},
	}
}

func (h *hermesAdapter) getConfigPath() string {
	return expandHome("~/.hermes/config.yaml")
}

func (h *hermesAdapter) getEnvPath() string {
	return expandHome("~/.hermes/.env")
}

func (h *hermesAdapter) Detect() (Status, error) {
	path := h.getConfigPath()
	installed := false

	cmdName := "which"
	if runtime.GOOS == "windows" {
		cmdName = "where"
	}
	if err := exec.Command(cmdName, "hermes").Run(); err == nil {
		installed = true
	} else if _, err := os.Stat(path); err == nil {
		installed = true
	}

	pointed := false
	if installed {
		m, err := readYAMLMap(path)
		if err == nil {
			if modelSec, ok := m["model"].(map[string]any); ok {
				if provider, ok := modelSec["provider"].(string); ok && provider == "custom" {
					if baseURL, ok := modelSec["base_url"].(string); ok && (strings.Contains(baseURL, "localhost") || strings.Contains(baseURL, "127.0.0.1") || strings.Contains(baseURL, "tinyroute")) {
						pointed = true
					}
				}
			}
		}
	}

	return Status{
		Installed:          installed,
		PointedAtTinyRoute: pointed,
		ConfigPath:         path,
	}, nil
}

func (h *hermesAdapter) Apply(input ApplyInput) (Result, error) {
	path := h.getConfigPath()
	envPath := h.getEnvPath()

	bak, err := backup(path)
	if err != nil {
		return Result{}, err
	}
	bakEnv, _ := backup(envPath)

	baseURL := input.BaseURL
	if !strings.HasSuffix(baseURL, "/v1") && !strings.HasSuffix(baseURL, "/v1/") {
		baseURL = strings.TrimRight(baseURL, "/") + "/v1"
	}

	modelID := input.Model
	if modelID == "" {
		modelID = "gpt-4o"
	}

	setFields := map[string]any{
		"model": map[string]any{
			"default":  modelID,
			"provider": "custom",
			"base_url": baseURL,
		},
	}

	if err := updateYAMLMap(path, setFields); err != nil {
		return Result{}, err
	}

	// Update .env file with OPENAI_API_KEY
	envContent := ""
	if data, err := os.ReadFile(envPath); err == nil {
		envContent = string(data)
	}

	envLine := fmt.Sprintf("OPENAI_API_KEY=%s\n", input.APIKey)
	lines := strings.Split(envContent, "\n")
	found := false
	for i, l := range lines {
		if strings.HasPrefix(l, "OPENAI_API_KEY=") {
			lines[i] = fmt.Sprintf("OPENAI_API_KEY=%s", input.APIKey)
			found = true
			break
		}
	}
	if !found {
		if envContent != "" && !strings.HasSuffix(envContent, "\n") {
			envContent += "\n"
		}
		envContent += envLine
	} else {
		envContent = strings.Join(lines, "\n")
	}

	if err := atomicWrite(envPath, []byte(envContent), 0600); err != nil {
		return Result{}, err
	}

	bakResult := bak
	if bakResult == "" {
		bakResult = bakEnv
	}

	return Result{
		Files:  []string{path, envPath},
		Key:    input.APIKey,
		Backup: bakResult,
	}, nil
}

func (h *hermesAdapter) Reset() error {
	path := h.getConfigPath()
	envPath := h.getEnvPath()

	_ = resetYAMLKeys(path, []string{"model"})

	if data, err := os.ReadFile(envPath); err == nil {
		lines := strings.Split(string(data), "\n")
		var newLines []string
		for _, l := range lines {
			if strings.HasPrefix(l, "OPENAI_API_KEY=") {
				continue
			}
			newLines = append(newLines, l)
		}
		_ = atomicWrite(envPath, []byte(strings.Join(newLines, "\n")), 0600)
	}

	return nil
}
