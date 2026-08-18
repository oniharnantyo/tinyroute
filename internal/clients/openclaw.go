package clients

import (
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type openclawAdapter struct{}

func init() {
	Register(&openclawAdapter{})
}

func (o *openclawAdapter) ID() string       { return "openclaw" }
func (o *openclawAdapter) Name() string     { return "Open Claw" }
func (o *openclawAdapter) Dialect() string  { return "openai" }
func (o *openclawAdapter) NeedsModel() bool { return true }

func (o *openclawAdapter) ModelSlots() []ModelSlot {
	return []ModelSlot{
		{ID: "model", Name: "Default Model", Kind: SlotSingle, Required: true},
	}
}

func (o *openclawAdapter) getSettingsPath() string {
	return expandHome("~/.openclaw/openclaw.json")
}

func (o *openclawAdapter) Detect() (Status, error) {
	path := o.getSettingsPath()
	installed := false

	cmdName := "which"
	if runtime.GOOS == "windows" {
		cmdName = "where"
	}
	if err := exec.Command(cmdName, "openclaw").Run(); err == nil {
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
			if models, ok := m["models"].(map[string]any); ok {
				if providers, ok := models["providers"].(map[string]any); ok {
					if tr, ok := providers["tinyroute"].(map[string]any); ok {
						pointed = true
						if bu, ok := tr["baseUrl"].(string); ok && bu != "" {
							currentBaseURL = bu
						}
						if k, ok := tr["apiKey"].(string); ok && k != "" {
							rawKey = k
							maskedKey = MaskKey(k)
						}
					}
				}
			}
			if agents, ok := m["agents"].(map[string]any); ok {
				if defaults, ok := agents["defaults"].(map[string]any); ok {
					if modelObj, ok := defaults["model"].(map[string]any); ok {
						if primary, ok := modelObj["primary"].(string); ok && primary != "" {
							modelID := strings.TrimPrefix(primary, "tinyroute/")
							slotValues["model"] = modelID
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
		CurrentBaseURL:     currentBaseURL,
		MaskedKey:          maskedKey,
		RawKey:             rawKey,
		SlotValues:         slotValues,
	}, nil
}

func (o *openclawAdapter) Apply(input ApplyInput) (Result, error) {
	path := o.getSettingsPath()

	bak, err := backup(path)
	if err != nil {
		return Result{}, err
	}

	baseURL := input.BaseURL
	if !strings.HasSuffix(baseURL, "/v1") && !strings.HasSuffix(baseURL, "/v1/") {
		baseURL = strings.TrimRight(baseURL, "/") + "/v1"
	}

	modelID := input.Model
	if modelID == "" {
		modelID = "gpt-4o"
	}

	m, err := readJSONMap(path)
	if err != nil {
		m = make(map[string]any)
	}

	apiKey := input.APIKey
	if apiKey == "" {
		if modelsMap, ok := m["models"].(map[string]any); ok {
			if providersMap, ok := modelsMap["providers"].(map[string]any); ok {
				if tr, ok := providersMap["tinyroute"].(map[string]any); ok {
					if k, ok := tr["apiKey"].(string); ok && k != "" {
						apiKey = k
					}
				}
			}
		}
	}

	fullModelID := "tinyroute/" + modelID

	// Structure: models.providers["tinyroute"]
	modelsMap, ok := m["models"].(map[string]any)
	if !ok || modelsMap == nil {
		modelsMap = make(map[string]any)
	}
	providersMap, ok := modelsMap["providers"].(map[string]any)
	if !ok || providersMap == nil {
		providersMap = make(map[string]any)
	}

	providersMap["tinyroute"] = map[string]any{
		"baseUrl": baseURL,
		"apiKey":  apiKey,
		"api":     "openai-completions",
		"models": []map[string]any{
			{"id": modelID, "name": modelID},
		},
	}
	modelsMap["providers"] = providersMap
	m["models"] = modelsMap

	// Structure: agents.defaults
	agentsMap, ok := m["agents"].(map[string]any)
	if !ok || agentsMap == nil {
		agentsMap = make(map[string]any)
	}
	defaultsMap, ok := agentsMap["defaults"].(map[string]any)
	if !ok || defaultsMap == nil {
		defaultsMap = make(map[string]any)
	}
	defaultsMap["model"] = map[string]any{
		"primary": fullModelID,
	}

	allowModels, ok := defaultsMap["models"].(map[string]any)
	if !ok || allowModels == nil {
		allowModels = make(map[string]any)
	}
	// Clean old tinyroute keys
	for k := range allowModels {
		if strings.HasPrefix(k, "tinyroute/") {
			delete(allowModels, k)
		}
	}
	allowModels[fullModelID] = map[string]any{}
	defaultsMap["models"] = allowModels
	agentsMap["defaults"] = defaultsMap
	m["agents"] = agentsMap

	if err := atomicWrite(path, mustMarshalJSON(m), 0600); err != nil {
		return Result{}, err
	}

	return Result{
		Files:  []string{path},
		Key:    apiKey,
		Backup: bak,
	}, nil
}

func (o *openclawAdapter) Reset() error {
	path := o.getSettingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}

	if modelsMap, ok := m["models"].(map[string]any); ok {
		if providersMap, ok := modelsMap["providers"].(map[string]any); ok {
			delete(providersMap, "tinyroute")
			if len(providersMap) == 0 {
				delete(modelsMap, "providers")
			}
		}
	}

	if agentsMap, ok := m["agents"].(map[string]any); ok {
		if defaultsMap, ok := agentsMap["defaults"].(map[string]any); ok {
			if allowModels, ok := defaultsMap["models"].(map[string]any); ok {
				for k := range allowModels {
					if strings.HasPrefix(k, "tinyroute/") {
						delete(allowModels, k)
					}
				}
			}
			if primaryObj, ok := defaultsMap["model"].(map[string]any); ok {
				if primary, ok := primaryObj["primary"].(string); ok && strings.HasPrefix(primary, "tinyroute/") {
					delete(defaultsMap, "model")
				}
			}
		}
	}

	return atomicWrite(path, mustMarshalJSON(m), 0600)
}
