package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeAdapter(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	adapter := &claudeAdapter{}

	if adapter.ID() != "claude" || adapter.Dialect() != "anthropic" || !adapter.NeedsModel() {
		t.Fatalf("unexpected adapter metadata")
	}

	// 1. Detect when missing
	st, err := adapter.Detect()
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if st.PointedAtTinyRoute {
		t.Errorf("expected not pointed at tinyroute when config missing")
	}

	// Pre-create existing settings
	settingsPath := adapter.getSettingsPath()
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	initialData := []byte(`{
  "theme": "light",
  "env": {
    "CUSTOM_ENV": "123"
  }
}`)
	if err := os.WriteFile(settingsPath, initialData, 0600); err != nil {
		t.Fatalf("write initial settings failed: %v", err)
	}

	// 2. Apply (merge-into-existing)
	res, err := adapter.Apply(ApplyInput{
		BaseURL: "http://127.0.0.1:8080/anthropic",
		APIKey:  "tr_live_testkey123",
	})
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if len(res.Files) != 1 || res.Files[0] != settingsPath {
		t.Errorf("unexpected files in result: %v", res.Files)
	}
	if res.Backup == "" {
		t.Errorf("expected backup file path")
	}

	// Verify updated config
	m, err := readJSONMap(settingsPath)
	if err != nil {
		t.Fatalf("read settings error: %v", err)
	}
	if m["theme"] != "light" {
		t.Errorf("expected theme=light preserved, got %v", m["theme"])
	}
	if m["hasCompletedOnboarding"] != true {
		t.Errorf("expected hasCompletedOnboarding=true, got %v", m["hasCompletedOnboarding"])
	}

	envMap := m["env"].(map[string]any)
	if envMap["CUSTOM_ENV"] != "123" {
		t.Errorf("expected CUSTOM_ENV=123 preserved")
	}
	if envMap["ANTHROPIC_BASE_URL"] != "http://127.0.0.1:8080/anthropic" {
		t.Errorf("expected ANTHROPIC_BASE_URL without /v1, got %v", envMap["ANTHROPIC_BASE_URL"])
	}
	if envMap["ANTHROPIC_AUTH_TOKEN"] != "tr_live_testkey123" {
		t.Errorf("expected ANTHROPIC_AUTH_TOKEN set, got %v", envMap["ANTHROPIC_AUTH_TOKEN"])
	}

	// 3. Detect when installed and pointed
	st, err = adapter.Detect()
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if !st.PointedAtTinyRoute {
		t.Errorf("expected pointed at tinyroute to be true")
	}

	// 4. Idempotent re-apply
	_, err = adapter.Apply(ApplyInput{
		BaseURL: "http://127.0.0.1:8080/anthropic/v1",
		APIKey:  "tr_live_testkey123",
	})
	if err != nil {
		t.Fatalf("Idempotent Apply error: %v", err)
	}

	// 5. Scoped reset
	if err := adapter.Reset(); err != nil {
		t.Fatalf("Reset error: %v", err)
	}

	mReset, err := readJSONMap(settingsPath)
	if err != nil {
		t.Fatalf("read settings after reset error: %v", err)
	}
	envReset := mReset["env"].(map[string]any)
	if envReset["CUSTOM_ENV"] != "123" {
		t.Errorf("expected CUSTOM_ENV=123 preserved after reset")
	}
	if _, ok := envReset["ANTHROPIC_BASE_URL"]; ok {
		t.Errorf("expected ANTHROPIC_BASE_URL removed after reset")
	}
	if _, ok := envReset["ANTHROPIC_AUTH_TOKEN"]; ok {
		t.Errorf("expected ANTHROPIC_AUTH_TOKEN removed after reset")
	}
}

func TestClaudeTiersWriting(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	a := &claudeAdapter{}
	_, err := a.Apply(ApplyInput{
		BaseURL: "http://localhost:8080/anthropic",
		APIKey:  "tr_live_claude",
		ModelSlots: map[string]string{
			"opus":  "claude-3-opus",
			"haiku": "claude-3-haiku",
		},
	})
	if err != nil {
		t.Fatalf("Apply claude: %v", err)
	}

	settingsPath := expandHome("~/.claude/settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "ANTHROPIC_DEFAULT_OPUS_MODEL") {
		t.Errorf("missing OPUS tier model")
	}
	if !strings.Contains(content, "ANTHROPIC_DEFAULT_HAIKU_MODEL") {
		t.Errorf("missing HAIKU tier model")
	}
	if strings.Contains(content, "ANTHROPIC_DEFAULT_SONNET_MODEL") {
		t.Errorf("unselected SONNET tier model should not be written")
	}
}
