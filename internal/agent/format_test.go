package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONEnvHelpers(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "settings.json")

	// Pre-populate with user settings
	initialJSON := []byte(`{
  "theme": "dark",
  "env": {
    "USER_CUSTOM_VAR": "keep_me"
  }
}`)
	if err := os.WriteFile(file, initialJSON, 0600); err != nil {
		t.Fatalf("failed to write initial JSON: %v", err)
	}

	// Update with new env and root fields
	err := updateJSONEnv(file, "env", map[string]any{
		"ANTHROPIC_BASE_URL": "http://localhost:8080/anthropic/v1",
	}, map[string]any{
		"hasCompletedOnboarding": true,
	})
	if err != nil {
		t.Fatalf("updateJSONEnv failed: %v", err)
	}

	m, err := readJSONMap(file)
	if err != nil {
		t.Fatalf("readJSONMap failed: %v", err)
	}

	if m["theme"] != "dark" {
		t.Errorf("expected theme=dark preserved, got %v", m["theme"])
	}
	if m["hasCompletedOnboarding"] != true {
		t.Errorf("expected hasCompletedOnboarding=true, got %v", m["hasCompletedOnboarding"])
	}
	envMap, ok := m["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected env to be a map, got %T", m["env"])
	}
	if envMap["USER_CUSTOM_VAR"] != "keep_me" {
		t.Errorf("expected USER_CUSTOM_VAR=keep_me preserved, got %v", envMap["USER_CUSTOM_VAR"])
	}
	if envMap["ANTHROPIC_BASE_URL"] != "http://localhost:8080/anthropic/v1" {
		t.Errorf("expected ANTHROPIC_BASE_URL updated, got %v", envMap["ANTHROPIC_BASE_URL"])
	}

	// Reset injected fields
	err = resetJSONEnv(file, "env", []string{"ANTHROPIC_BASE_URL"}, []string{"hasCompletedOnboarding"})
	if err != nil {
		t.Fatalf("resetJSONEnv failed: %v", err)
	}

	mReset, err := readJSONMap(file)
	if err != nil {
		t.Fatalf("readJSONMap after reset failed: %v", err)
	}
	if _, ok := mReset["hasCompletedOnboarding"]; ok {
		t.Errorf("expected hasCompletedOnboarding to be removed")
	}
	envReset, _ := mReset["env"].(map[string]any)
	if envReset["USER_CUSTOM_VAR"] != "keep_me" {
		t.Errorf("expected USER_CUSTOM_VAR=keep_me preserved after reset, got %v", envReset["USER_CUSTOM_VAR"])
	}
	if _, ok := envReset["ANTHROPIC_BASE_URL"]; ok {
		t.Errorf("expected ANTHROPIC_BASE_URL to be removed after reset")
	}
}

func TestTOMLProviderHelpers(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.toml")

	initialTOML := []byte(`
model = "gpt-4"

[other_section]
foo = "bar"
`)
	if err := os.WriteFile(file, initialTOML, 0600); err != nil {
		t.Fatalf("failed to write initial TOML: %v", err)
	}

	err := updateTOMLProvider(file, "model_providers", "tinyroute", map[string]any{
		"base_url": "http://localhost:8080/openai/v1",
		"wire_api": "responses",
	}, map[string]any{
		"model_provider": "tinyroute",
	})
	if err != nil {
		t.Fatalf("updateTOMLProvider failed: %v", err)
	}

	m, err := readTOMLMap(file)
	if err != nil {
		t.Fatalf("readTOMLMap failed: %v", err)
	}

	if m["model"] != "gpt-4" {
		t.Errorf("expected model=gpt-4 preserved, got %v", m["model"])
	}
	if m["model_provider"] != "tinyroute" {
		t.Errorf("expected model_provider=tinyroute, got %v", m["model_provider"])
	}
	other, ok := m["other_section"].(map[string]any)
	if !ok || other["foo"] != "bar" {
		t.Errorf("expected other_section.foo=bar preserved")
	}

	// Reset
	err = resetTOMLProvider(file, "model_providers", "tinyroute", []string{"model_provider"})
	if err != nil {
		t.Fatalf("resetTOMLProvider failed: %v", err)
	}

	mReset, err := readTOMLMap(file)
	if err != nil {
		t.Fatalf("readTOMLMap after reset failed: %v", err)
	}
	if _, ok := mReset["model_provider"]; ok {
		t.Errorf("expected model_provider removed after reset")
	}
	if _, ok := mReset["model_providers"]; ok {
		t.Errorf("expected model_providers section removed after reset")
	}
}

func TestYAMLMergeHelpers(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")

	initialYAML := []byte(`
user_pref: true
model: old-model
`)
	if err := os.WriteFile(file, initialYAML, 0600); err != nil {
		t.Fatalf("failed to write initial YAML: %v", err)
	}

	err := updateYAMLMap(file, map[string]any{
		"model":    "gpt-4o",
		"endpoint": "http://localhost:8080/openai/v1",
	})
	if err != nil {
		t.Fatalf("updateYAMLMap failed: %v", err)
	}

	m, err := readYAMLMap(file)
	if err != nil {
		t.Fatalf("readYAMLMap failed: %v", err)
	}

	if m["user_pref"] != true {
		t.Errorf("expected user_pref=true preserved, got %v", m["user_pref"])
	}
	if m["model"] != "gpt-4o" {
		t.Errorf("expected model=gpt-4o updated, got %v", m["model"])
	}
	if m["endpoint"] != "http://localhost:8080/openai/v1" {
		t.Errorf("expected endpoint set, got %v", m["endpoint"])
	}

	// Reset
	err = resetYAMLKeys(file, []string{"endpoint"})
	if err != nil {
		t.Fatalf("resetYAMLKeys failed: %v", err)
	}

	mReset, err := readYAMLMap(file)
	if err != nil {
		t.Fatalf("readYAMLMap after reset failed: %v", err)
	}
	if _, ok := mReset["endpoint"]; ok {
		t.Errorf("expected endpoint removed after reset")
	}
	if mReset["user_pref"] != true {
		t.Errorf("expected user_pref=true preserved after reset")
	}
}
