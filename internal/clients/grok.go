package clients

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

type grokAdapter struct{}

func init() {
	Register(&grokAdapter{})
}

func (g *grokAdapter) ID() string       { return "grok" }
func (g *grokAdapter) Name() string     { return "Grok Build" }
func (g *grokAdapter) Dialect() string  { return "openai" }
func (g *grokAdapter) NeedsModel() bool { return true }

func (g *grokAdapter) ModelSlots() []ModelSlot {
	return []ModelSlot{
		{ID: "model", Name: "Primary Model", Kind: SlotSingle, Required: true},
		{ID: "subagent", Name: "Subagent Model", Kind: SlotSingle, Required: false},
	}
}

func (g *grokAdapter) getConfigPath() string {
	return expandHome("~/.grok/config.toml")
}

func (g *grokAdapter) Detect() (Status, error) {
	path := g.getConfigPath()
	installed := false

	cmdName := "which"
	if runtime.GOOS == "windows" {
		cmdName = "where"
	}
	if err := exec.Command(cmdName, "grok").Run(); err == nil {
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
		m, err := readTOMLMap(path)
		if err == nil {
			if modelSec, ok := m["model"].(map[string]any); ok {
				if baseURL, ok := modelSec["base_url"].(string); ok && baseURL != "" {
					currentBaseURL = baseURL
					pointed = true
				}
				if key, ok := modelSec["api_key"].(string); ok && key != "" {
					rawKey = key
					maskedKey = MaskKey(key)
				}
				if model, ok := modelSec["model"].(string); ok && model != "" {
					slotValues["model"] = model
				}
			}
			if agentsSec, ok := m["agents"].(map[string]any); ok {
				if subSec, ok := agentsSec["subagent"].(map[string]any); ok {
					if model, ok := subSec["model"].(string); ok && model != "" {
						slotValues["subagent"] = model
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

func (g *grokAdapter) Apply(input ApplyInput) (Result, error) {
	path := g.getConfigPath()

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

	m, err := readTOMLMap(path)
	if err != nil {
		m = make(map[string]any)
	}

	apiKey := input.APIKey
	if apiKey == "" {
		if modelSec, ok := m["model"].(map[string]any); ok {
			if k, ok := modelSec["api_key"].(string); ok && k != "" {
				apiKey = k
			}
		}
	}

	modelFields := map[string]any{
		"base_url": baseURL,
		"api_key":  apiKey,
		"model":    modelID,
	}

	m["model"] = modelFields

	subagentFields := map[string]any{
		"model": modelID,
	}
	var agentsSec map[string]any
	if existing, ok := m["agents"].(map[string]any); ok && existing != nil {
		agentsSec = existing
	} else {
		agentsSec = make(map[string]any)
	}
	agentsSec["subagent"] = subagentFields
	m["agents"] = agentsSec

	data, err := toml.Marshal(m)
	if err != nil {
		return Result{}, err
	}

	if err := atomicWrite(path, data, 0600); err != nil {
		return Result{}, err
	}

	return Result{
		Files:  []string{path},
		Key:    apiKey,
		Backup: bak,
	}, nil
}

func (g *grokAdapter) Reset() error {
	path := g.getConfigPath()
	_ = resetTOMLProvider(path, "model", "", nil)
	_ = resetTOMLProvider(path, "", "", []string{"model"})
	_ = resetTOMLProvider(path, "agents", "subagent", nil)
	return nil
}
