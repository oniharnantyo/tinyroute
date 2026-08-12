package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type copilotAdapter struct{}

func init() {
	Register(&copilotAdapter{})
}

func (c *copilotAdapter) ID() string       { return "copilot" }
func (c *copilotAdapter) Name() string     { return "GitHub Copilot CLI" }
func (c *copilotAdapter) Dialect() string  { return "openai" }
func (c *copilotAdapter) NeedsModel() bool { return true }

func (c *copilotAdapter) ModelSlots() []ModelSlot {
	return []ModelSlot{
		{ID: "models", Name: "Model List", Kind: SlotMulti, Required: true},
	}
}

func (c *copilotAdapter) getConfigPath() string {
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData != "" {
			return filepath.Join(appData, "Code", "User", "chatLanguageModels.json")
		}
		return expandHome("~/AppData/Roaming/Code/User/chatLanguageModels.json")
	}
	if runtime.GOOS == "darwin" {
		return expandHome("~/Library/Application Support/Code/User/chatLanguageModels.json")
	}
	return expandHome("~/.config/Code/User/chatLanguageModels.json")
}

func (c *copilotAdapter) Detect() (Status, error) {
	path := c.getConfigPath()
	installed := false

	if _, err := os.Stat(path); err == nil {
		installed = true
	}

	pointed := false
	if installed {
		data, err := os.ReadFile(path)
		if err == nil {
			var list []map[string]any
			if json.Unmarshal(data, &list) == nil {
				for _, entry := range list {
					if name, ok := entry["name"].(string); ok && name == "tinyroute" {
						pointed = true
						break
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

func (c *copilotAdapter) Apply(input ApplyInput) (Result, error) {
	path := c.getConfigPath()

	bak, err := backup(path)
	if err != nil {
		return Result{}, err
	}

	baseURL := input.BaseURL
	if !strings.HasSuffix(baseURL, "/v1") && !strings.HasSuffix(baseURL, "/v1/") {
		baseURL = strings.TrimRight(baseURL, "/") + "/v1"
	}
	endpointURL := fmt.Sprintf("%s/chat/completions", strings.TrimRight(baseURL, "/"))

	var list []map[string]any
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &list)
	}

	modelsList := input.Models
	if len(modelsList) == 0 {
		mID := input.Model
		if mID == "" {
			mID = "gpt-4o"
		}
		modelsList = []string{mID}
	}

	var modelsEntries []map[string]any
	for _, id := range modelsList {
		if id == "" {
			continue
		}
		modelsEntries = append(modelsEntries, map[string]any{
			"id":              id,
			"name":            id,
			"url":             endpointURL,
			"toolCalling":     true,
			"vision":          false,
			"maxInputTokens":  128000,
			"maxOutputTokens": 16000,
		})
	}

	newEntry := map[string]any{
		"name":   "tinyroute",
		"vendor": "customendpoint",
		"apiKey": input.APIKey,
		"models": modelsEntries,
	}

	replaced := false
	for i, entry := range list {
		if name, ok := entry["name"].(string); ok && name == "tinyroute" {
			list[i] = newEntry
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, newEntry)
	}

	if err := atomicWrite(path, mustMarshalJSON(list), 0600); err != nil {
		return Result{}, err
	}

	return Result{
		Files:  []string{path},
		Key:    input.APIKey,
		Backup: bak,
	}, nil
}

func (c *copilotAdapter) Reset() error {
	path := c.getConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var list []map[string]any
	if err := json.Unmarshal(data, &list); err != nil {
		return nil
	}

	newList := make([]map[string]any, 0, len(list))
	for _, entry := range list {
		if name, ok := entry["name"].(string); ok && name == "tinyroute" {
			continue
		}
		newList = append(newList, entry)
	}

	return atomicWrite(path, mustMarshalJSON(newList), 0600)
}
