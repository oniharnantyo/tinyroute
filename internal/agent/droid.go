package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type droidAdapter struct{}

func init() {
	Register(&droidAdapter{})
}

func (d *droidAdapter) ID() string       { return "droid" }
func (d *droidAdapter) Name() string     { return "Factory Droid" }
func (d *droidAdapter) Dialect() string  { return "openai" }
func (d *droidAdapter) NeedsModel() bool { return true }

func (d *droidAdapter) ModelSlots() []ModelSlot {
	return []ModelSlot{
		{ID: "models", Name: "Model List", Kind: SlotMulti, Required: true},
		{ID: "active", Name: "Active Model", Kind: SlotSingle, Required: false},
	}
}

func (d *droidAdapter) getSettingsPath() string {
	return expandHome("~/.factory/settings.json")
}

func (d *droidAdapter) Detect() (Status, error) {
	path := d.getSettingsPath()
	installed := false

	cmdName := "which"
	if runtime.GOOS == "windows" {
		cmdName = "where"
	}
	if err := exec.Command(cmdName, "droid").Run(); err == nil {
		installed = true
	} else if _, err := os.Stat(path); err == nil {
		installed = true
	}

	pointed := false
	if installed {
		m, err := readJSONMap(path)
		if err == nil {
			if customModels, ok := m["customModels"].([]any); ok {
				for _, cm := range customModels {
					if modelObj, ok := cm.(map[string]any); ok {
						if id, ok := modelObj["id"].(string); ok && strings.HasPrefix(id, "custom:tinyroute") {
							pointed = true
							break
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

func (d *droidAdapter) Apply(input ApplyInput) (Result, error) {
	path := d.getSettingsPath()

	bak, err := backup(path)
	if err != nil {
		return Result{}, err
	}

	baseURL := input.BaseURL
	if !strings.HasSuffix(baseURL, "/v1") && !strings.HasSuffix(baseURL, "/v1/") {
		baseURL = strings.TrimRight(baseURL, "/") + "/v1"
	}

	m, err := readJSONMap(path)
	if err != nil {
		m = make(map[string]any)
	}

	var customModels []map[string]any
	if existing, ok := m["customModels"].([]any); ok {
		for _, item := range existing {
			if modelObj, ok := item.(map[string]any); ok {
				if id, ok := modelObj["id"].(string); ok && strings.HasPrefix(id, "custom:tinyroute") {
					continue
				}
				customModels = append(customModels, modelObj)
			}
		}
	}

	modelsList := input.Models
	if len(modelsList) == 0 {
		mID := input.Model
		if mID == "" {
			mID = "gpt-4o"
		}
		modelsList = []string{mID}
	}

	activeModel := ""
	if input.ModelSlots != nil {
		activeModel = input.ModelSlots["active"]
	}

	var newEntries []map[string]any
	for i, mID := range modelsList {
		if mID == "" {
			continue
		}
		newEntries = append(newEntries, map[string]any{
			"model":           mID,
			"id":              fmt.Sprintf("custom:tinyroute-%d", i),
			"index":           i,
			"baseUrl":         baseURL,
			"apiKey":          input.APIKey,
			"displayName":     mID,
			"maxOutputTokens": 131072,
			"noImageSupport":  false,
			"provider":        "openai",
		})
	}

	// Re-order if activeModel specified
	if activeModel != "" {
		activeIdx := -1
		for idx, entry := range newEntries {
			if entry["model"] == activeModel {
				activeIdx = idx
				break
			}
		}
		if activeIdx > 0 {
			act := newEntries[activeIdx]
			newEntries = append(newEntries[:activeIdx], newEntries[activeIdx+1:]...)
			newEntries = append([]map[string]any{act}, newEntries...)
			for i := range newEntries {
				newEntries[i]["index"] = i
			}
		}
	}

	customModels = append(newEntries, customModels...)
	m["customModels"] = customModels

	if err := atomicWrite(path, mustMarshalJSON(m), 0600); err != nil {
		return Result{}, err
	}

	return Result{
		Files:  []string{path},
		Key:    input.APIKey,
		Backup: bak,
	}, nil
}

func (d *droidAdapter) Reset() error {
	path := d.getSettingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}

	if existing, ok := m["customModels"].([]any); ok {
		var newModels []map[string]any
		for _, item := range existing {
			if modelObj, ok := item.(map[string]any); ok {
				if id, ok := modelObj["id"].(string); ok && strings.HasPrefix(id, "custom:tinyroute") {
					continue
				}
				newModels = append(newModels, modelObj)
			}
		}
		if len(newModels) == 0 {
			delete(m, "customModels")
		} else {
			m["customModels"] = newModels
		}
	}

	return atomicWrite(path, mustMarshalJSON(m), 0600)
}
