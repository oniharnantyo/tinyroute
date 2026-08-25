package clients

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/oniharnantyo/tinyroute/internal/auth"
	"github.com/oniharnantyo/tinyroute/internal/config"
)

type KeyStrategy string

const (
	KeyStrategyMint  KeyStrategy = "mint"
	KeyStrategyReuse KeyStrategy = "reuse"
)

type InstallRequest struct {
	ClientID      string            `json:"client_id"`
	BaseURL       string            `json:"base_url,omitempty"`
	APIKey        string            `json:"api_key,omitempty"`
	KeyStrategy   KeyStrategy       `json:"key_strategy,omitempty"`
	KeyName       string            `json:"key_name,omitempty"`
	Model         string            `json:"model,omitempty"`
	Models        []string          `json:"models,omitempty"`
	ModelSlots    map[string]string `json:"model_slots,omitempty"`
	ContextWindow string            `json:"context_window,omitempty"`
}

type Plan struct {
	Client        Client            `json:"-"`
	ClientID      string            `json:"client_id"`
	ClientName    string            `json:"client_name"`
	Dialect       string            `json:"dialect"`
	BaseURL       string            `json:"base_url"`
	APIKey        string            `json:"api_key,omitempty"`
	KeyStrategy   KeyStrategy       `json:"key_strategy"`
	KeyName       string            `json:"key_name,omitempty"`
	Model         string            `json:"model,omitempty"`
	Models        []string          `json:"models,omitempty"`
	ModelSlots    map[string]string `json:"model_slots,omitempty"`
	ContextWindow string            `json:"context_window,omitempty"`
	ConfigPath    string            `json:"config_path"`
	HasBackup     bool              `json:"has_backup"`
	BackupPath    string            `json:"backup_path,omitempty"`
}

type InstallResult struct {
	Files  []string `json:"files"`
	Key    string   `json:"key,omitempty"`
	Backup string   `json:"backup,omitempty"`
}

type Installer struct {
	listenAddr string
	keysPath   string
}

func NewInstaller(listenAddr string, keysPath string) *Installer {
	return &Installer{
		listenAddr: listenAddr,
		keysPath:   keysPath,
	}
}

func DialectBaseURL(listen, dialect string) string {
	listen = strings.TrimSpace(listen)
	scheme := "http://"
	if strings.HasPrefix(listen, "http://") {
		scheme = ""
	} else if strings.HasPrefix(listen, "https://") {
		scheme = ""
	}
	if strings.HasPrefix(listen, ":") {
		listen = "127.0.0.1" + listen
	} else if strings.HasPrefix(listen, "0.0.0.0:") {
		listen = "127.0.0.1" + listen[7:]
	}
	if scheme != "" {
		listen = scheme + listen
	}
	listen = strings.TrimRight(listen, "/")

	switch dialect {
	case "openai", "openairesponses", "openai-responses":
		return listen + "/openai/v1"
	case "anthropic":
		return listen + "/anthropic"
	case "gemini":
		return listen + "/gemini"
	default:
		return listen + "/" + strings.TrimPrefix(dialect, "/")
	}
}

// DiscoverModelsForDialect returns every whitelisted model across all
// configured providers, in "provider:model" form — the ID the strict prefix
// router accepts. The inbound dialect does not filter the list: translation
// makes any provider's models reachable from any dialect's routes.
func DiscoverModelsForDialect(_ string) []string {
	svc, err := config.LoadService()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return nil
	}
	topo, err := config.ParseTopology(data)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var result []string
	for name, prov := range topo.Providers {
		for _, m := range prov.Models {
			if m == "" {
				continue
			}
			id := name + ":" + m
			if !seen[id] {
				seen[id] = true
				result = append(result, id)
			}
		}
	}
	for _, cb := range topo.Combos {
		if cb.Name != "" {
			key := "combo:" + cb.Name
			if !seen[key] {
				seen[key] = true
				result = append(result, key)
			}
		}
	}
	sort.Strings(result)
	return result
}

func (inst *Installer) Plan(req InstallRequest) (*Plan, error) {
	c, ok := Get(req.ClientID)
	if !ok {
		return nil, fmt.Errorf("unknown client %q", req.ClientID)
	}

	baseURL := req.BaseURL
	if baseURL == "" {
		listen := inst.listenAddr
		if listen == "" {
			if svc, err := config.LoadService(); err == nil {
				listen = svc.Listen
			}
		}
		if listen == "" {
			listen = "127.0.0.1:8080"
		}
		baseURL = DialectBaseURL(listen, c.Dialect())
	}

	st, _ := c.Detect()

	keyStrategy := req.KeyStrategy
	if keyStrategy == "" {
		if req.APIKey != "" || (st.PointedAtTinyRoute && st.RawKey != "") {
			keyStrategy = KeyStrategyReuse
		} else {
			keyStrategy = KeyStrategyMint
		}
	}

	apiKey := req.APIKey
	if apiKey == "" && keyStrategy == KeyStrategyReuse && st.RawKey != "" {
		apiKey = st.RawKey
	}

	keyName := req.KeyName
	if keyName == "" && keyStrategy == KeyStrategyMint {
		keyName = "client-" + c.ID()
	}

	slotsMap := make(map[string]string)
	for k, v := range req.ModelSlots {
		slotsMap[k] = v
	}

	primaryModel := req.Model
	if primaryModel == "" {
		if val, exists := slotsMap["model"]; exists && val != "" {
			primaryModel = val
		} else if val, exists := slotsMap["models"]; exists && val != "" {
			primaryModel = val
		}
	}
	if c.NeedsModel() {
		for _, slot := range c.ModelSlots() {
			if val, exists := slotsMap[slot.ID]; exists && val != "" {
				if primaryModel == "" {
					primaryModel = val
				}
			} else if primaryModel != "" && (slot.ID == "model" || slot.ID == "models") {
				slotsMap[slot.ID] = primaryModel
			} else if primaryModel == "" && slot.Required {
				primaryModel = "gpt-4o"
				slotsMap[slot.ID] = primaryModel
			}
		}
	}

	hasBackup := false
	backupPath := ""
	if st.ConfigPath != "" {
		backupPath = st.ConfigPath + ".tinyroute.bak"
		if _, err := os.Stat(st.ConfigPath); err == nil {
			hasBackup = true
		}
	}

	return &Plan{
		Client:        c,
		ClientID:      c.ID(),
		ClientName:    c.Name(),
		Dialect:       c.Dialect(),
		BaseURL:       baseURL,
		APIKey:        apiKey,
		KeyStrategy:   keyStrategy,
		KeyName:       keyName,
		Model:         primaryModel,
		Models:        req.Models,
		ModelSlots:    slotsMap,
		ContextWindow: req.ContextWindow,
		ConfigPath:    st.ConfigPath,
		HasBackup:     hasBackup,
		BackupPath:    backupPath,
	}, nil
}

func (inst *Installer) Apply(plan *Plan) (*InstallResult, error) {
	if plan == nil {
		return nil, fmt.Errorf("plan cannot be nil")
	}

	c := plan.Client
	if c == nil {
		var ok bool
		c, ok = Get(plan.ClientID)
		if !ok {
			return nil, fmt.Errorf("unknown client %q", plan.ClientID)
		}
	}

	apiKey := plan.APIKey
	mintedKey := ""

	if plan.KeyStrategy == KeyStrategyMint {
		var err error
		mintedKey, err = inst.MintKey(c.Dialect(), plan.KeyName)
		if err != nil {
			return nil, fmt.Errorf("mint key for %s: %w", c.ID(), err)
		}
		apiKey = mintedKey
	} else if apiKey == "" {
		st, _ := c.Detect()
		if st.RawKey != "" {
			apiKey = st.RawKey
		}
	}

	res, err := c.Apply(ApplyInput{
		BaseURL:       plan.BaseURL,
		APIKey:        apiKey,
		Model:         plan.Model,
		Models:        plan.Models,
		ModelSlots:    plan.ModelSlots,
		ContextWindow: plan.ContextWindow,
	})
	if err != nil {
		return nil, fmt.Errorf("apply configuration for %s: %w", c.ID(), err)
	}

	outKey := apiKey
	if mintedKey != "" {
		outKey = mintedKey
	}

	return &InstallResult{
		Files:  res.Files,
		Key:    outKey,
		Backup: res.Backup,
	}, nil
}

func (inst *Installer) MintKey(dialect, keyName string) (string, error) {
	keysPath := inst.keysPath
	if keysPath == "" {
		svc, err := config.LoadService()
		if err != nil {
			return "", fmt.Errorf("load service config: %w", err)
		}
		keysPath = svc.KeysPath
	}

	kf := auth.KeyFile{}
	if data, err := os.ReadFile(keysPath); err == nil {
		_ = json.Unmarshal(data, &kf)
	}

	if keyName == "" {
		keyName = "client-" + dialect
	}

	plaintext, keyRecord, err := auth.GenerateKey(keyName)
	if err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}

	kf.Keys = append(kf.Keys, keyRecord)

	if err := auth.WriteKeyFile(keysPath, kf); err != nil {
		return "", fmt.Errorf("write key file %s: %w", keysPath, err)
	}

	return plaintext, nil
}
