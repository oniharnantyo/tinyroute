package agent

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type clineAdapter struct{}

func init() {
	Register(&clineAdapter{})
}

func (c *clineAdapter) ID() string       { return "cline" }
func (c *clineAdapter) Name() string     { return "Cline" }
func (c *clineAdapter) Dialect() string  { return "openai" }
func (c *clineAdapter) NeedsModel() bool { return true }

func (c *clineAdapter) ModelSlots() []ModelSlot {
	return []ModelSlot{
		{ID: "model", Name: "Primary Model", Kind: SlotSingle, Required: true},
	}
}

func (c *clineAdapter) getGlobalStatePath() string {
	return expandHome("~/.cline/data/globalState.json")
}

func (c *clineAdapter) getSecretsPath() string {
	return expandHome("~/.cline/data/secrets.json")
}

func (c *clineAdapter) Detect() (Status, error) {
	gsPath := c.getGlobalStatePath()
	installed := false

	cmdName := "which"
	if runtime.GOOS == "windows" {
		cmdName = "where"
	}
	if err := exec.Command(cmdName, "cline").Run(); err == nil {
		installed = true
	} else if _, err := os.Stat(gsPath); err == nil {
		installed = true
	}

	pointed := false
	if installed {
		m, err := readJSONMap(gsPath)
		if err == nil {
			if provider, ok := m["actModeApiProvider"].(string); ok && provider == "openai" {
				if baseURL, ok := m["openAiBaseUrl"].(string); ok && (strings.Contains(baseURL, "localhost") || strings.Contains(baseURL, "127.0.0.1") || strings.Contains(baseURL, "tinyroute")) {
					pointed = true
				}
			}
		}
	}

	return Status{
		Installed:          installed,
		PointedAtTinyRoute: pointed,
		ConfigPath:         gsPath,
	}, nil
}

func (c *clineAdapter) Apply(input ApplyInput) (Result, error) {
	gsPath := c.getGlobalStatePath()
	secPath := c.getSecretsPath()

	bakGS, err := backup(gsPath)
	if err != nil {
		return Result{}, err
	}
	bakSec, _ := backup(secPath)

	baseURL := input.BaseURL
	if strings.HasSuffix(baseURL, "/v1") {
		baseURL = baseURL[:len(baseURL)-3]
	} else if strings.HasSuffix(baseURL, "/v1/") {
		baseURL = baseURL[:len(baseURL)-4]
	}
	baseURL = strings.TrimRight(baseURL, "/")

	gsFields := map[string]any{
		"actModeApiProvider":    "openai",
		"planModeApiProvider":   "openai",
		"openAiBaseUrl":         baseURL,
		"openAiModelId":         input.Model,
		"planModeOpenAiModelId": input.Model,
	}

	if err := updateJSONEnv(gsPath, "", nil, gsFields); err != nil {
		return Result{}, err
	}

	secFields := map[string]any{
		"openAiApiKey": input.APIKey,
	}
	if err := updateJSONEnv(secPath, "", nil, secFields); err != nil {
		return Result{}, err
	}

	bak := bakGS
	if bak == "" {
		bak = bakSec
	}

	return Result{
		Files:  []string{gsPath, secPath},
		Key:    input.APIKey,
		Backup: bak,
	}, nil
}

func (c *clineAdapter) Reset() error {
	gsPath := c.getGlobalStatePath()
	secPath := c.getSecretsPath()

	m, err := readJSONMap(gsPath)
	if err == nil && m["actModeApiProvider"] == "openai" {
		delete(m, "openAiBaseUrl")
		delete(m, "openAiModelId")
		delete(m, "planModeOpenAiModelId")
		m["actModeApiProvider"] = "cline"
		m["planModeApiProvider"] = "cline"
		_ = atomicWrite(gsPath, mustMarshalJSON(m), 0600)
	}

	_ = resetJSONEnv(secPath, "", nil, []string{"openAiApiKey"})
	return nil
}
