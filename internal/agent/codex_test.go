package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodexAdapter(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	adapter := &codexAdapter{}

	if adapter.ID() != "codex" || adapter.Dialect() != "openai-responses" || !adapter.NeedsModel() {
		t.Fatalf("unexpected adapter metadata")
	}

	// 1. Detect missing
	st, err := adapter.Detect()
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if st.PointedAtTinyRoute {
		t.Errorf("expected not pointed at tinyroute when missing")
	}

	// Pre-create existing config.toml
	configPath := adapter.getConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	initialTOML := []byte(`
user_setting = "abc"
`)
	if err := os.WriteFile(configPath, initialTOML, 0600); err != nil {
		t.Fatalf("write initial config failed: %v", err)
	}

	// 2. Apply (create/merge)
	res, err := adapter.Apply(ApplyInput{
		BaseURL: "http://127.0.0.1:8080/openai",
		APIKey:  "tr_live_codexkey",
		Model:   "gpt-4o",
	})
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if len(res.Files) != 2 {
		t.Errorf("expected 2 files modified (config.toml and auth.json), got %v", res.Files)
	}

	// Verify config.toml
	m, err := readTOMLMap(configPath)
	if err != nil {
		t.Fatalf("read config.toml error: %v", err)
	}
	if m["user_setting"] != "abc" {
		t.Errorf("expected user_setting=abc preserved")
	}
	if m["model"] != "gpt-4o" {
		t.Errorf("expected model=gpt-4o, got %v", m["model"])
	}
	if m["model_provider"] != "tinyroute" {
		t.Errorf("expected model_provider=tinyroute, got %v", m["model_provider"])
	}

	sec := m["model_providers"].(map[string]any)
	trProv := sec["tinyroute"].(map[string]any)
	if trProv["base_url"] != "http://127.0.0.1:8080/openai/v1" {
		t.Errorf("expected normalized base_url, got %v", trProv["base_url"])
	}
	if trProv["wire_api"] != "responses" {
		t.Errorf("expected wire_api=responses, got %v", trProv["wire_api"])
	}

	// Verify auth.json
	authPath := adapter.getAuthPath()
	mAuth, err := readJSONMap(authPath)
	if err != nil {
		t.Fatalf("read auth.json error: %v", err)
	}
	if mAuth["OPENAI_API_KEY"] != "tr_live_codexkey" {
		t.Errorf("expected OPENAI_API_KEY set, got %v", mAuth["OPENAI_API_KEY"])
	}
	if mAuth["auth_mode"] != "apikey" {
		t.Errorf("expected auth_mode=apikey, got %v", mAuth["auth_mode"])
	}

	// 3. Detect when pointed
	st, err = adapter.Detect()
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if !st.PointedAtTinyRoute {
		t.Errorf("expected pointed at tinyroute to be true")
	}

	// 4. Reset
	if err := adapter.Reset(); err != nil {
		t.Fatalf("Reset error: %v", err)
	}

	mReset, err := readTOMLMap(configPath)
	if err != nil {
		t.Fatalf("read config after reset error: %v", err)
	}
	if mReset["user_setting"] != "abc" {
		t.Errorf("expected user_setting=abc preserved after reset")
	}
	if _, ok := mReset["model_provider"]; ok {
		t.Errorf("expected model_provider removed after reset")
	}

	mAuthReset, _ := readJSONMap(authPath)
	if _, ok := mAuthReset["OPENAI_API_KEY"]; ok {
		t.Errorf("expected OPENAI_API_KEY removed after reset")
	}
}
