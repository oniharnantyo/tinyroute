package agent

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type deepseekAdapter struct{}

func init() {
	Register(&deepseekAdapter{})
}

func (d *deepseekAdapter) ID() string       { return "deepseek" }
func (d *deepseekAdapter) Name() string     { return "DeepSeek TUI" }
func (d *deepseekAdapter) Dialect() string  { return "openai" }
func (d *deepseekAdapter) NeedsModel() bool { return true }

func (d *deepseekAdapter) ModelSlots() []ModelSlot {
	return []ModelSlot{
		{ID: "model", Name: "Primary Model", Kind: SlotSingle, Required: true},
	}
}

func (d *deepseekAdapter) getConfigPath() string {
	return expandHome("~/.deepseek/config.toml")
}

func (d *deepseekAdapter) Detect() (Status, error) {
	path := d.getConfigPath()
	installed := false

	cmdName := "which"
	if runtime.GOOS == "windows" {
		cmdName = "where"
	}
	if err := exec.Command(cmdName, "deepseek").Run(); err == nil {
		installed = true
	} else if _, err := os.Stat(path); err == nil {
		installed = true
	}

	pointed := false
	if installed {
		m, err := readTOMLMap(path)
		if err == nil {
			if provider, ok := m["provider"].(string); ok && provider == "openai" {
				if sec, ok := m["providers"].(map[string]any); ok {
					if openaiSec, ok := sec["openai"].(map[string]any); ok {
						if baseURL, ok := openaiSec["base_url"].(string); ok && (strings.Contains(baseURL, "localhost") || strings.Contains(baseURL, "127.0.0.1") || strings.Contains(baseURL, "tinyroute")) {
							pointed = true
						}
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

func (d *deepseekAdapter) Apply(input ApplyInput) (Result, error) {
	path := d.getConfigPath()

	bak, err := backup(path)
	if err != nil {
		return Result{}, err
	}

	baseURL := input.BaseURL
	if !strings.HasSuffix(baseURL, "/v1") && !strings.HasSuffix(baseURL, "/v1/") {
		baseURL = strings.TrimRight(baseURL, "/") + "/v1"
	}

	rootFields := map[string]any{
		"provider": "openai",
	}

	providerFields := map[string]any{
		"base_url": baseURL,
		"api_key":  input.APIKey,
		"model":    input.Model,
	}

	if err := updateTOMLProvider(path, "providers", "openai", providerFields, rootFields); err != nil {
		return Result{}, err
	}

	return Result{
		Files:  []string{path},
		Key:    input.APIKey,
		Backup: bak,
	}, nil
}

func (d *deepseekAdapter) Reset() error {
	path := d.getConfigPath()
	defaultTOML := []byte("provider = \"deepseek\"\n")
	return atomicWrite(path, defaultTOML, 0600)
}
