package clients

import (
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

type opencodeAdapter struct{}

func init() {
	Register(&opencodeAdapter{})
}

func (o *opencodeAdapter) ID() string       { return "opencode" }
func (o *opencodeAdapter) Name() string     { return "OpenCode" }
func (o *opencodeAdapter) Dialect() string  { return "openai" }
func (o *opencodeAdapter) NeedsModel() bool { return true }

func (o *opencodeAdapter) ModelSlots() []ModelSlot {
	return []ModelSlot{
		{ID: "model", Name: "Primary Model", Kind: SlotSingle, Required: true},
		{ID: "subagent", Name: "Subagent Model", Kind: SlotSingle, Required: false},
	}
}

func (o *opencodeAdapter) getConfigPath() string {
	return expandHome("~/.config/opencode/opencode.json")
}

func (o *opencodeAdapter) Detect() (Status, error) {
	path := o.getConfigPath()
	installed := false

	cmdName := "which"
	if runtime.GOOS == "windows" {
		cmdName = "where"
	}
	if err := exec.Command(cmdName, "opencode").Run(); err == nil {
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
			if providerMap, ok := m["provider"].(map[string]any); ok {
				if tr, ok := providerMap["tinyroute"].(map[string]any); ok {
					pointed = true
					if opts, ok := tr["options"].(map[string]any); ok {
						if bu, ok := opts["baseURL"].(string); ok && bu != "" {
							currentBaseURL = bu
						}
						if k, ok := opts["apiKey"].(string); ok && k != "" {
							rawKey = k
							maskedKey = MaskKey(k)
						}
					}
					if modelsMap, ok := tr["models"].(map[string]any); ok {
						var mNames []string
						for k := range modelsMap {
							mNames = append(mNames, k)
						}
						if len(mNames) > 0 {
							sort.Strings(mNames)
							slotValues["models"] = strings.Join(mNames, ", ")
							slotValues["model"] = mNames[0]
						}
					}
				}
			}
			if modelVal, ok := m["model"].(string); ok && modelVal != "" {
				trimmed := strings.TrimPrefix(modelVal, "tinyroute/")
				slotValues["model"] = trimmed
				if slotValues["models"] == "" {
					slotValues["models"] = trimmed
				}
			}
			if agentMap, ok := m["agent"].(map[string]any); ok {
				if explorer, ok := agentMap["explorer"].(map[string]any); ok {
					if subModel, ok := explorer["model"].(string); ok && subModel != "" {
						slotValues["subagent"] = strings.TrimPrefix(subModel, "tinyroute/")
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

func (o *opencodeAdapter) Apply(input ApplyInput) (Result, error) {
	path := o.getConfigPath()

	bak, err := backup(path)
	if err != nil {
		return Result{}, err
	}

	baseURL := input.BaseURL
	if !strings.HasSuffix(baseURL, "/v1") && !strings.HasSuffix(baseURL, "/v1/") {
		baseURL = strings.TrimRight(baseURL, "/") + "/v1"
	}

	modelID := input.Model
	if modelID == "" && input.ModelSlots != nil {
		if m := input.ModelSlots["model"]; m != "" {
			modelID = m
		} else if m := input.ModelSlots["models"]; m != "" {
			modelID = m
		}
	}
	modelID = strings.TrimPrefix(modelID, "tinyroute/")
	if modelID == "" {
		modelID = "gpt-4o"
	}

	subagentModel := ""
	if input.ModelSlots != nil {
		subagentModel = strings.TrimPrefix(input.ModelSlots["subagent"], "tinyroute/")
	}

	m, err := readJSONMap(path)
	if err != nil {
		m = make(map[string]any)
	}

	providerMap, ok := m["provider"].(map[string]any)
	if !ok || providerMap == nil {
		providerMap = make(map[string]any)
	}

	modelsList := input.Models
	if len(modelsList) == 0 && input.ModelSlots != nil && input.ModelSlots["models"] != "" {
		parts := strings.Split(input.ModelSlots["models"], ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			p = strings.TrimPrefix(p, "tinyroute/")
			if p != "" {
				modelsList = append(modelsList, p)
			}
		}
	}
	if len(modelsList) == 0 {
		modelsList = []string{modelID}
	}
	if subagentModel != "" {
		found := false
		for _, name := range modelsList {
			if name == subagentModel {
				found = true
				break
			}
		}
		if !found {
			modelsList = append(modelsList, subagentModel)
		}
	}

	modelsMap := make(map[string]any)
	for _, mName := range modelsList {
		if mName == "" {
			continue
		}
		modelsMap[mName] = map[string]any{
			"name": mName,
			"modalities": map[string]any{
				"input":  []string{"text", "image"},
				"output": []string{"text"},
			},
		}
	}

	apiKey := input.APIKey
	if apiKey == "" {
		if tr, ok := providerMap["tinyroute"].(map[string]any); ok {
			if opts, ok := tr["options"].(map[string]any); ok {
				if k, ok := opts["apiKey"].(string); ok && k != "" {
					apiKey = k
				}
			}
		}
	}

	providerMap["tinyroute"] = map[string]any{
		"npm": "@ai-sdk/openai-compatible",
		"options": map[string]any{
			"baseURL": baseURL,
			"apiKey":  apiKey,
		},
		"models": modelsMap,
	}
	m["provider"] = providerMap
	m["model"] = "tinyroute/" + modelID

	if subagentModel != "" {
		agentMap, ok := m["agent"].(map[string]any)
		if !ok || agentMap == nil {
			agentMap = make(map[string]any)
		}
		agentMap["explorer"] = map[string]any{
			"description": "Fast explorer subagent for codebase exploration",
			"mode":        "subagent",
			"model":       "tinyroute/" + subagentModel,
		}
		m["agent"] = agentMap
	}

	if err := atomicWrite(path, mustMarshalJSON(m), 0600); err != nil {
		return Result{}, err
	}

	return Result{
		Files:  []string{path},
		Key:    apiKey,
		Backup: bak,
	}, nil
}

func (o *opencodeAdapter) Reset() error {
	path := o.getConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}

	if providerMap, ok := m["provider"].(map[string]any); ok {
		delete(providerMap, "tinyroute")
		if len(providerMap) == 0 {
			delete(m, "provider")
		}
	}

	if modelStr, ok := m["model"].(string); ok && strings.HasPrefix(modelStr, "tinyroute/") {
		delete(m, "model")
	}

	if agentMap, ok := m["agent"].(map[string]any); ok {
		if explorer, ok := agentMap["explorer"].(map[string]any); ok {
			if modelStr, ok := explorer["model"].(string); ok && strings.HasPrefix(modelStr, "tinyroute/") {
				delete(agentMap, "explorer")
			}
		}
		if len(agentMap) == 0 {
			delete(m, "agent")
		}
	}

	return atomicWrite(path, mustMarshalJSON(m), 0600)
}
