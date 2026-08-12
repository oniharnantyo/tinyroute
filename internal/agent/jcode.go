package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type jcodeAdapter struct{}

func init() {
	Register(&jcodeAdapter{})
}

func (j *jcodeAdapter) ID() string       { return "jcode" }
func (j *jcodeAdapter) Name() string     { return "jcode" }
func (j *jcodeAdapter) Dialect() string  { return "openai" }
func (j *jcodeAdapter) NeedsModel() bool { return true }

func (j *jcodeAdapter) ModelSlots() []ModelSlot {
	return []ModelSlot{
		{ID: "models", Name: "Model List", Kind: SlotMulti, Required: true},
	}
}

func (j *jcodeAdapter) getConfigPath() string {
	return expandHome("~/.jcode/config.toml")
}

func (j *jcodeAdapter) getEnvPath() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		configDir = expandHome("~/.config")
	}
	return filepath.Join(configDir, "jcode", "provider-tinyroute.env")
}

func (j *jcodeAdapter) Detect() (Status, error) {
	path := j.getConfigPath()
	installed := false

	cmdName := "which"
	if runtime.GOOS == "windows" {
		cmdName = "where"
	}
	if err := exec.Command(cmdName, "jcode").Run(); err == nil {
		installed = true
	} else if _, err := os.Stat(path); err == nil {
		installed = true
	}

	pointed := false
	if installed {
		m, err := readTOMLMap(path)
		if err == nil {
			if providers, ok := m["providers"].(map[string]any); ok {
				if _, ok := providers["tinyroute"]; ok {
					pointed = true
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

func (j *jcodeAdapter) Apply(input ApplyInput) (Result, error) {
	path := j.getConfigPath()
	envPath := j.getEnvPath()

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
	if len(input.Models) > 0 && input.Models[0] != "" {
		modelID = input.Models[0]
	}
	if modelID == "" {
		modelID = "gpt-4o"
	}

	providerFields := map[string]any{
		"type":             "openai-compatible",
		"base_url":         baseURL,
		"auth":             "bearer",
		"api_key_env":      "JCODE_TINYROUTE_API_KEY",
		"env_file":         "provider-tinyroute.env",
		"default_model":    modelID,
		"requires_api_key": true,
	}

	if err := updateTOMLProvider(path, "providers", "tinyroute", providerFields, nil); err != nil {
		return Result{}, err
	}

	envContent := fmt.Sprintf("# jcode provider environment variables\nJCODE_TINYROUTE_API_KEY=\"%s\"\n", input.APIKey)
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

func (j *jcodeAdapter) Reset() error {
	path := j.getConfigPath()
	envPath := j.getEnvPath()

	_ = resetTOMLProvider(path, "providers", "tinyroute", nil)
	_ = os.Remove(envPath)
	return nil
}
