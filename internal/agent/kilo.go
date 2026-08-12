package agent

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type kiloAdapter struct{}

func init() {
	Register(&kiloAdapter{})
}

func (k *kiloAdapter) ID() string       { return "kilo" }
func (k *kiloAdapter) Name() string     { return "Kilo Code" }
func (k *kiloAdapter) Dialect() string  { return "openai" }
func (k *kiloAdapter) NeedsModel() bool { return true }

func (k *kiloAdapter) ModelSlots() []ModelSlot {
	return []ModelSlot{
		{ID: "model", Name: "Default Model", Kind: SlotSingle, Required: true},
	}
}

func (k *kiloAdapter) getAuthPath() string {
	return expandHome("~/.local/share/kilo/auth.json")
}

func (k *kiloAdapter) getKiloConfigPath() string {
	return expandHome("~/.config/kilo/kilo.json")
}

func (k *kiloAdapter) Detect() (Status, error) {
	path := k.getAuthPath()
	kiloConfigPath := k.getKiloConfigPath()
	installed := false

	cmdName := "which"
	if runtime.GOOS == "windows" {
		cmdName = "where"
	}
	if err := exec.Command(cmdName, "kilo").Run(); err == nil {
		installed = true
	} else if _, err := os.Stat(path); err == nil {
		installed = true
	} else if _, err := os.Stat(kiloConfigPath); err == nil {
		installed = true
	}

	pointed := false
	if installed {
		m, err := readJSONMap(path)
		if err == nil {
			if entry, ok := m["openai-compatible"].(map[string]any); ok {
				if baseURL, ok := entry["baseUrl"].(string); ok && (strings.Contains(baseURL, "localhost") || strings.Contains(baseURL, "127.0.0.1") || strings.Contains(baseURL, "tinyroute")) {
					pointed = true
				}
			}
		}
		if !pointed {
			km, err := readJSONMap(kiloConfigPath)
			if err == nil {
				if providers, ok := km["provider"].(map[string]any); ok {
					if tr, ok := providers["tinyroute"].(map[string]any); ok {
						if baseURL, ok := tr["baseUrl"].(string); ok && baseURL != "" {
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

func (k *kiloAdapter) Apply(input ApplyInput) (Result, error) {
	path := k.getAuthPath()
	kiloConfigPath := k.getKiloConfigPath()

	bak, err := backup(path)
	if err != nil {
		return Result{}, err
	}
	bakKilo, _ := backup(kiloConfigPath)

	baseURL := input.BaseURL
	if !strings.HasSuffix(baseURL, "/v1") && !strings.HasSuffix(baseURL, "/v1/") {
		baseURL = strings.TrimRight(baseURL, "/") + "/v1"
	}

	modelID := input.Model
	if modelID == "" {
		modelID = "gpt-4o"
	}

	// 1. Update auth.json
	authMap, err := readJSONMap(path)
	if err != nil {
		authMap = make(map[string]any)
	}

	authMap["openai-compatible"] = map[string]any{
		"type":    "api-key",
		"apiKey":  input.APIKey,
		"baseUrl": baseURL,
		"model":   modelID,
	}

	if err := atomicWrite(path, mustMarshalJSON(authMap), 0600); err != nil {
		return Result{}, err
	}

	// 2. Update kilo.json provider block
	kiloMap, err := readJSONMap(kiloConfigPath)
	if err != nil {
		kiloMap = make(map[string]any)
	}
	providerBlock, ok := kiloMap["provider"].(map[string]any)
	if !ok || providerBlock == nil {
		providerBlock = make(map[string]any)
	}
	providerBlock["tinyroute"] = map[string]any{
		"name":    "tinyroute",
		"type":    "openai-compatible",
		"baseUrl": baseURL,
		"apiKey":  input.APIKey,
		"models":  []string{modelID},
	}
	kiloMap["provider"] = providerBlock
	_ = atomicWrite(kiloConfigPath, mustMarshalJSON(kiloMap), 0600)

	bakResult := bak
	if bakResult == "" {
		bakResult = bakKilo
	}

	return Result{
		Files:  []string{path, kiloConfigPath},
		Key:    input.APIKey,
		Backup: bakResult,
	}, nil
}

func (k *kiloAdapter) Reset() error {
	path := k.getAuthPath()
	kiloConfigPath := k.getKiloConfigPath()

	_ = resetJSONEnv(path, "", nil, []string{"openai-compatible", "tinyroute"})
	_ = resetJSONEnv(kiloConfigPath, "provider", []string{"tinyroute"}, nil)

	return nil
}
