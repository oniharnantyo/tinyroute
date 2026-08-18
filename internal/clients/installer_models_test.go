package clients

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeTopology(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tinyroute.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write topology: %v", err)
	}
	t.Setenv("TINYROUTE_CONFIG", path)
	return path
}

func TestDiscoverModelsForDialect_ListsAllWhitelistedModels(t *testing.T) {
	writeTopology(t, `
providers:
  grok:
    dialect: openai
    base_url: https://api.x.ai/v1
    models: [grok-4, grok-4-fast]
  openrouter:
    dialect: openai
    base_url: https://openrouter.ai/api/v1
    models: [anthropic/claude-opus-4]
routes:
  - from: anthropic
    match: "*"
    chain: [grok:$model]
`)

	got := DiscoverModelsForDialect("anthropic")
	want := []string{"grok:grok-4", "grok:grok-4-fast", "openrouter:anthropic/claude-opus-4"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DiscoverModelsForDialect() = %v, want %v", got, want)
	}
}

func TestDiscoverModelsForDialect_NoWhitelistedModels_ReturnsEmpty(t *testing.T) {
	writeTopology(t, `
providers:
  grok:
    dialect: openai
    base_url: https://api.x.ai/v1
routes:
  - from: anthropic
    match: "*"
    chain: [grok:$model]
`)

	if got := DiscoverModelsForDialect("anthropic"); len(got) != 0 {
		t.Errorf("DiscoverModelsForDialect() = %v, want empty", got)
	}
}

func TestDiscoverModelsForDialect_MissingConfig_ReturnsNil(t *testing.T) {
	t.Setenv("TINYROUTE_CONFIG", filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if got := DiscoverModelsForDialect("anthropic"); got != nil {
		t.Errorf("DiscoverModelsForDialect() = %v, want nil", got)
	}
}
