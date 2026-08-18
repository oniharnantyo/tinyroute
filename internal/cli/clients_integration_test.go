package cli

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/auth"
	"github.com/oniharnantyo/tinyroute/internal/cli/interactive"
	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/dialect"
	_ "github.com/oniharnantyo/tinyroute/internal/dialect/anthropic"
	"github.com/oniharnantyo/tinyroute/internal/proxy"
	"github.com/oniharnantyo/tinyroute/internal/route"
)

func TestEndToEndAgentInstallClaude(t *testing.T) {
	// 1. Start mock upstream Anthropic server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") == "" && r.Header.Get("authorization") == "" {
			http.Error(w, "missing auth header", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "msg_test_123",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "hello from mock anthropic"}],
			"model": "claude-3-5-sonnet-20241022",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 5, "output_tokens": 5}
		}`))
	}))
	defer upstreamServer.Close()

	// 2. Setup isolated test environment
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dotDir := filepath.Join(tmpHome, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir .tinyroute: %v", err)
	}

	configPath := filepath.Join(dotDir, "config.yaml")
	keysPath := filepath.Join(dotDir, "keys.json")

	// Save initial config.yaml with route to mock upstream
	configYAML := []byte(strings.ReplaceAll(`providers:
  mock-anthropic:
    dialect: anthropic
    base_url: UPSTREAM_URL
routes:
- from: anthropic
  match: '*'
  chain:
  - mock-anthropic:$model`, "UPSTREAM_URL", upstreamServer.URL))
	if err := os.WriteFile(configPath, configYAML, 0600); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	// Save initial keys.json
	if err := os.WriteFile(keysPath, []byte(`{"keys":[]}`), 0600); err != nil {
		t.Fatalf("write keys.json: %v", err)
	}

	// 3. Start tinyroute HTTP proxy server
	topoWatcher, err := config.NewWatcher(configPath, config.ParseTopology)
	if err != nil {
		t.Fatalf("new topology watcher: %v", err)
	}
	keyWatcher, err := config.NewWatcher(keysPath, auth.ParseKeyFile)
	if err != nil {
		t.Fatalf("new key watcher: %v", err)
	}

	clock := route.RealClock{}
	health := route.NewHealthStore(clock, filepath.Join(dotDir, "state.json"))

	getRouter := func() (*route.Router, error) {
		topo := topoWatcher.Get()
		if topo == nil {
			return nil, http.ErrHandlerTimeout
		}
		rawRoutes := make([]route.RawRoute, 0, len(topo.Routes))
		for _, r := range topo.Routes {
			rawRoutes = append(rawRoutes, route.RawRoute{
				From:  r.From,
				Match: r.Match,
				Chain: r.Chain,
			})
		}
		entries, err := route.ParseRoutes(rawRoutes)
		if err != nil {
			return nil, err
		}
		return route.New(entries, topo.Providers), nil
	}

	getProvider := func(name string) (proxy.ProviderInfo, bool) {
		topo := topoWatcher.Get()
		if topo == nil {
			return proxy.ProviderInfo{}, false
		}
		p, ok := topo.Providers[name]
		if !ok {
			return proxy.ProviderInfo{}, false
		}
		return proxy.ProviderInfo{
			Dialect: p.Dialect,
			BaseURL: p.BaseURL,
			APIKey:  "mock-upstream-key",
		}, true
	}

	getDialect := func(name string) (core.Dialect, bool) {
		return dialect.ByName(name)
	}

	deps := &proxy.Deps{
		Logger:      slog.Default(),
		Transport:   &http.Transport{},
		GetProvider: getProvider,
		GetDialect:  getDialect,
		Health:      health,
		Selector:    &route.OrderedSelector{},
		Recorder:    nil,
		CaptureMode: "metadata",
		Cooldown429: 10 * time.Second,
		Cooldown5xx: 30 * time.Second,
	}

	handler := proxy.Handler(deps)
	rateLimiter := auth.NewRateLimiter(func(string) *auth.RateSpec { return nil })

	mux := http.NewServeMux()
	for _, name := range dialect.Names() {
		d, ok := dialect.ByName(name)
		if !ok {
			continue
		}
		for _, p := range d.MountPaths() {
			mux.Handle(p, requestHandler(d, slog.Default(), keyWatcher, rateLimiter, getRouter, handler))
		}
	}

	proxyServer := httptest.NewServer(mux)
	defer proxyServer.Close()

	// Configure TINYROUTE_LISTEN for LoadService
	serverHostPort := strings.TrimPrefix(proxyServer.URL, "http://")
	t.Setenv("TINYROUTE_LISTEN", serverHostPort)
	t.Setenv("TINYROUTE_CONFIG", configPath)
	t.Setenv("TINYROUTE_KEYS", keysPath)

	// 4. Run `agent install claude` in non-interactive mode
	falseVal := false
	interactive.SetCanPromptOverride(&falseVal)
	defer interactive.SetCanPromptOverride(nil)

	err = cmdClientInstall([]string{"claude", "--no-interactive"})
	if err != nil {
		t.Fatalf("cmdClientInstall claude failed: %v", err)
	}

	// 5. Read configured settings from ~/.claude/settings.json
	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read ~/.claude/settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}

	envMap, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected env object in settings.json")
	}

	baseURL, _ := envMap["ANTHROPIC_BASE_URL"].(string)
	authToken, _ := envMap["ANTHROPIC_AUTH_TOKEN"].(string)

	if baseURL == "" || authToken == "" {
		t.Fatalf("missing ANTHROPIC_BASE_URL or ANTHROPIC_AUTH_TOKEN in settings.json")
	}

	// 6. Send Claude-Code-shaped POST request to configured endpoint
	// Claude Code appends /v1/messages to ANTHROPIC_BASE_URL
	reqURL := strings.TrimRight(baseURL, "/") + "/v1/messages"
	reqBody := []byte(`{
		"model": "claude-3-5-sonnet-20241022",
		"max_tokens": 10,
		"messages": [{"role": "user", "content": "hello"}]
	}`)

	req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("x-api-key", authToken)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send POST request to %s: %v", reqURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", resp.StatusCode)
	}
}
