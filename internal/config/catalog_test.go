package config

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseCatalog(t *testing.T) {
	jsonMap := []byte(`{
		"providers": {
			"openai": ["gpt-4o", "gpt-4-turbo"],
			"anthropic": ["claude-3-5-sonnet-20240620"]
		}
	}`)

	cat, err := ParseCatalog(jsonMap)
	if err != nil {
		t.Fatalf("ParseCatalog failed: %v", err)
	}

	if len(cat.Providers["openai"]) != 2 {
		t.Errorf("expected 2 openai models, got %d", len(cat.Providers["openai"]))
	}
	if len(cat.Providers["anthropic"]) != 1 {
		t.Errorf("expected 1 anthropic model, got %d", len(cat.Providers["anthropic"]))
	}

	jsonArray := []byte(`[
		{"id": "gpt-4o", "provider": "openai"},
		{"id": "claude-3-5-sonnet", "provider": "anthropic"}
	]`)

	cat2, err := ParseCatalog(jsonArray)
	if err != nil {
		t.Fatalf("ParseCatalog array failed: %v", err)
	}
	if len(cat2.Providers["openai"]) != 1 || cat2.Providers["openai"][0] != "gpt-4o" {
		t.Errorf("expected gpt-4o for openai, got %v", cat2.Providers["openai"])
	}
}

func TestLoadOrRefreshCatalog_CacheHit(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "api.json")
	checksumFile := filepath.Join(tmpDir, "api.json.sha256")

	data := []byte(`{"openai": ["gpt-4o"]}`)
	if err := os.WriteFile(cacheFile, data, 0600); err != nil {
		t.Fatal(err)
	}

	hash := sha256.Sum256(data)
	if err := os.WriteFile(checksumFile, []byte(hex.EncodeToString(hash[:])+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cat, err := LoadOrRefreshCatalog(tmpDir, "http://invalid-url-should-not-be-called.local")
	if err != nil {
		t.Fatalf("expected cache hit, got error: %v", err)
	}
	if len(cat.Providers["openai"]) != 1 || cat.Providers["openai"][0] != "gpt-4o" {
		t.Errorf("unexpected catalog content: %+v", cat)
	}
}

func TestLoadOrRefreshCatalog_FetchAndCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"openai": ["gpt-4o", "gpt-3.5-turbo"]}`))
	}))
	defer server.Close()

	tmpDir := t.TempDir()

	cat, err := LoadOrRefreshCatalog(tmpDir, server.URL)
	if err != nil {
		t.Fatalf("LoadOrRefreshCatalog failed: %v", err)
	}

	if len(cat.Providers["openai"]) != 2 {
		t.Errorf("expected 2 models, got %d", len(cat.Providers["openai"]))
	}

	// Verify cache file exists
	cacheFile := filepath.Join(tmpDir, "api.json")
	checksumFile := filepath.Join(tmpDir, "api.json.sha256")

	if _, err := os.Stat(cacheFile); err != nil {
		t.Errorf("cache file missing: %v", err)
	}
	if _, err := os.Stat(checksumFile); err != nil {
		t.Errorf("checksum file missing: %v", err)
	}
}

func TestFetchProviderModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"data": [
				{"id": "gpt-4o"},
				{"id": "gpt-4o-mini"}
			]
		}`))
	}))
	defer server.Close()

	models, err := FetchProviderModels(server.URL, "test-key", "openai")
	if err != nil {
		t.Fatalf("FetchProviderModels failed: %v", err)
	}
	if len(models) != 2 || models[0] != "gpt-4o" {
		t.Errorf("unexpected models: %v", models)
	}
}

func TestFetchProviderModels_Gemini(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models" {
			t.Errorf("expected gemini path /v1beta/models, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("expected Bearer auth, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		// Gemini /v1beta/models: names are "models/<id>" and only some support chat.
		w.Write([]byte(`{
			"models": [
				{"name": "models/gemini-2.5-flash", "supportedGenerationMethods": ["generateContent", "countTokens"]},
				{"name": "models/gemini-2.5-pro", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/text-embedding-004", "supportedGenerationMethods": ["embedContent"]}
			]
		}`))
	}))
	defer server.Close()

	models, err := FetchProviderModels(server.URL, "test-token", "gemini")
	if err != nil {
		t.Fatalf("FetchProviderModels failed: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 chat models (embedding filtered), got %d: %v", len(models), models)
	}
	want := map[string]bool{"gemini-2.5-flash": true, "gemini-2.5-pro": true}
	for _, m := range models {
		if !want[m] {
			t.Errorf("unexpected model %q (prefix not stripped or embedding not filtered)", m)
		}
	}
}

func TestFetchProviderModels_Cline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"recommended": [
				{"id": "anthropic/claude-opus-5"},
				{"id": "openai/gpt-5.6-sol"}
			],
			"free": [
				{"id": "deepseek/deepseek-v4-flash"}
			],
			"clinePass": [
				{"id": "cline-pass/qwen3.8-max"}
			]
		}`))
	}))
	defer server.Close()

	models, err := FetchProviderModels(server.URL, "test-token", "openai")
	if err != nil {
		t.Fatalf("FetchProviderModels failed: %v", err)
	}
	if len(models) != 4 {
		t.Fatalf("expected 4 models, got %d: %v", len(models), models)
	}
}
