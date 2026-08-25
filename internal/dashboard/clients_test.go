package dashboard_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/auth"
	_ "github.com/oniharnantyo/tinyroute/internal/clients"
	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/dashboard"
)

func setupTestDashboard(t *testing.T) (*http.ServeMux, string, string) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	keysPath := filepath.Join(tempDir, "keys.json")

	_ = os.WriteFile(configPath, []byte(`{"listen": "127.0.0.1:8787"}`), 0600)
	_ = auth.WriteKeyFile(keysPath, auth.KeyFile{})

	svc := config.Service{
		Listen:     "127.0.0.1:8787",
		ConfigPath: configPath,
		KeysPath:   keysPath,
	}

	pwStore, _ := dashboard.NewPasswordStore(filepath.Join(tempDir, "password.hash"))
	sessStore := dashboard.NewSessionStore()
	loginLimiter := dashboard.NewLoginLimiter()
	keyWatcher, _ := config.NewWatcher(keysPath, auth.ParseKeyFile)

	deps := &dashboard.Deps{
		Service:       svc,
		PasswordStore: pwStore,
		SessionStore:  sessStore,
		LoginLimiter:  loginLimiter,
		KeyWatcher:    keyWatcher,
	}

	mux := http.NewServeMux()
	dashboard.RegisterRoutes(mux, deps)

	sessID := sessStore.CreateSession(1 * time.Hour)
	return mux, sessID, tempDir
}

func TestDashboardClientsList(t *testing.T) {
	mux, sessID, _ := setupTestDashboard(t)

	req := httptest.NewRequest("GET", "/dashboard/clients", nil)
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /dashboard/clients, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Claude Code") {
		t.Errorf("expected Claude Code in dashboard clients view, body:\n%s", body)
	}
	if !strings.Contains(body, "Codex") {
		t.Errorf("expected Codex in dashboard clients view")
	}
}

func TestDashboardClientDetailView(t *testing.T) {
	mux, sessID, _ := setupTestDashboard(t)

	req := httptest.NewRequest("GET", "/dashboard/clients/claude", nil)
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /dashboard/clients/claude, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Claude Code") {
		t.Errorf("expected Claude Code in detail view")
	}
	if !strings.Contains(body, "127.0.0.1:8787/anthropic") {
		t.Errorf("expected endpoint 127.0.0.1:8787/anthropic in detail view")
	}

	// Header has client logo
	if !strings.Contains(body, "client-logo") && !strings.Contains(body, "<svg") {
		t.Errorf("expected client logo in detail view header")
	}

	// Header omits ID / Dialect subtitle
	if strings.Contains(body, "ID: claude") || strings.Contains(body, "Dialect: anthropic") {
		t.Errorf("expected no ID/Dialect subtitle in detail view header")
	}

	// Removed helper texts
	if strings.Contains(body, "Configure base URL and authentication for") {
		t.Errorf("expected no helper text under Endpoint & Key Configuration")
	}
	if strings.Contains(body, "Select default models for") {
		t.Errorf("expected no helper text under Model Slots")
	}

	// Removed Experimental Settings
	if strings.Contains(body, "Experimental Settings") || strings.Contains(body, "Filter Naming Requests") || strings.Contains(body, "Exa MCP") {
		t.Errorf("expected no Experimental Settings block in detail view")
	}

	// Layout and picker markup
	if !strings.Contains(body, "/components/shadcn-templ-") {
		t.Errorf("expected shadcn-templ component bundle script tag in layout")
	}
	if !strings.Contains(body, "dialog-slot-") || !strings.Contains(body, "data-tui-dialog") {
		t.Errorf("expected templui dialog modal triggers and bindings")
	}
	if !strings.Contains(body, "(None / Default)") {
		t.Errorf("expected (None / Default) option in picker modal")
	}
	if !strings.Contains(body, "Search models or providers") {
		t.Errorf("expected model search input in picker modal")
	}
	if !strings.Contains(body, "data-filter-input") {
		t.Errorf("expected data-filter-input binding in picker modal")
	}
}

func TestDashboardClientDetail_ComboPickerGrouping(t *testing.T) {
	mux, sessID, tempDir := setupTestDashboard(t)

	topoPath := filepath.Join(tempDir, "tinyroute.yaml")
	topoContent := `
providers:
  mock-anthropic:
    dialect: anthropic
    base_url: https://api.anthropic.com
    models:
    - claude-3-5-sonnet-20241022
combos:
- name: claude-fast
  members:
  - mock-anthropic:claude-3-5-sonnet-20241022
- name: claude-smart
  members:
  - mock-anthropic:claude-3-5-sonnet-20241022
`
	if err := os.WriteFile(topoPath, []byte(topoContent), 0600); err != nil {
		t.Fatalf("write topo: %v", err)
	}
	t.Setenv("TINYROUTE_CONFIG", topoPath)

	req := httptest.NewRequest("GET", "/dashboard/clients/claude", nil)
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /dashboard/clients/claude, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Assert "Combos" header is present
	if !strings.Contains(body, "Combos") {
		t.Errorf("expected 'Combos' header in picker dialog")
	}

	// Assert no combo renders under a "defaults" header
	if strings.Contains(body, ">defaults<") || strings.Contains(body, ">Defaults<") {
		t.Errorf("expected no defaults group header when all entries are prefixed")
	}

	// Assert combo entries render with combo:<name> values
	if !strings.Contains(body, `data-model-val="combo:claude-fast"`) {
		t.Errorf("expected data-model-val for combo:claude-fast")
	}
	if !strings.Contains(body, `data-model-val="combo:claude-smart"`) {
		t.Errorf("expected data-model-val for combo:claude-smart")
	}

	// Apply submission with combo key form
	form := url.Values{}
	form.Set("base_url", "http://127.0.0.1:8787/anthropic")
	form.Set("key_strategy", "mint")
	form.Set("slot_sonnet", "combo:claude-fast")

	reqApply := httptest.NewRequest("POST", "/dashboard/clients/claude/apply", strings.NewReader(form.Encode()))
	reqApply.Host = "127.0.0.1:8787"
	reqApply.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqApply.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	recApply := httptest.NewRecorder()

	mux.ServeHTTP(recApply, reqApply)

	if recApply.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /dashboard/clients/claude/apply, got %d", recApply.Code)
	}

	// Verify written settings.json contains combo:claude-fast verbatim
	claudeSettings := filepath.Join(tempDir, ".claude", "settings.json")
	data, err := os.ReadFile(claudeSettings)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}
	envMap, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected env in settings.json: %s", string(data))
	}
	if envMap["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "combo:claude-fast" {
		t.Errorf("expected ANTHROPIC_DEFAULT_SONNET_MODEL to be combo:claude-fast, got: %v", envMap["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
}

func TestDashboardClientPlan(t *testing.T) {
	mux, sessID, _ := setupTestDashboard(t)

	form := url.Values{}
	form.Set("base_url", "http://127.0.0.1:8787/anthropic")
	form.Set("key_strategy", "mint")

	req := httptest.NewRequest("POST", "/dashboard/clients/claude/plan", strings.NewReader(form.Encode()))
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /dashboard/clients/claude/plan, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"client_id":"claude"`) {
		t.Errorf("expected plan JSON to contain client_id claude, got: %s", body)
	}
}

func TestDashboardClientApplyAndReset(t *testing.T) {
	mux, sessID, tempDir := setupTestDashboard(t)

	// 1. Apply with mint strategy
	form := url.Values{}
	form.Set("base_url", "http://127.0.0.1:8787/anthropic")
	form.Set("key_strategy", "mint")

	req := httptest.NewRequest("POST", "/dashboard/clients/claude/apply", strings.NewReader(form.Encode()))
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 (one-time key reveal page) for Apply, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Minted API Key") || !strings.Contains(body, "tr_live_") {
		t.Errorf("expected one-time key reveal page with tr_live_ key, body:\n%s", body)
	}

	// Verify file was written
	claudeSettings := filepath.Join(tempDir, ".claude", "settings.json")
	if _, err := os.Stat(claudeSettings); err != nil {
		t.Errorf("expected .claude/settings.json to exist: %v", err)
	}

	// 2. Reset client config
	reqReset := httptest.NewRequest("POST", "/dashboard/clients/claude/reset", nil)
	reqReset.Host = "127.0.0.1:8787"
	reqReset.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	recReset := httptest.NewRecorder()

	mux.ServeHTTP(recReset, reqReset)

	if recReset.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303 SeeOther for Reset, got %d", recReset.Code)
	}
}

func TestDashboardClientApplyModelSlotContextSuffix(t *testing.T) {
	mux, sessID, tempDir := setupTestDashboard(t)

	// 1. Plan with a slot model carrying the [1m] extended context suffix
	formPlan := url.Values{}
	formPlan.Set("base_url", "http://127.0.0.1:8787/anthropic")
	formPlan.Set("key_strategy", "reuse")
	formPlan.Set("api_key", "custom-secret-key-123")
	formPlan.Set("slot_opus", "anthropic:claude-sonnet-4-5[1m]")

	reqPlan := httptest.NewRequest("POST", "/dashboard/clients/claude/plan", strings.NewReader(formPlan.Encode()))
	reqPlan.Host = "127.0.0.1:8787"
	reqPlan.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqPlan.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	recPlan := httptest.NewRecorder()

	mux.ServeHTTP(recPlan, reqPlan)
	if recPlan.Code != http.StatusOK {
		t.Fatalf("expected status 200 for plan, got %d", recPlan.Code)
	}
	if !strings.Contains(recPlan.Body.String(), `anthropic:claude-sonnet-4-5[1m]`) {
		t.Errorf("expected [1m]-suffixed slot value in plan JSON, got: %s", recPlan.Body.String())
	}

	// 2. Apply with reuse key strategy
	formApply := url.Values{}
	formApply.Set("base_url", "http://127.0.0.1:8787/anthropic")
	formApply.Set("key_strategy", "reuse")
	formApply.Set("api_key", "custom-secret-key-123")
	formApply.Set("slot_opus", "anthropic:claude-sonnet-4-5[1m]")

	reqApply := httptest.NewRequest("POST", "/dashboard/clients/claude/apply", strings.NewReader(formApply.Encode()))
	reqApply.Host = "127.0.0.1:8787"
	reqApply.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqApply.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	recApply := httptest.NewRecorder()

	mux.ServeHTTP(recApply, reqApply)
	if recApply.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303 SeeOther for Apply with reuse key, got %d", recApply.Code)
	}

	location := recApply.Header().Get("Location")
	if !strings.Contains(location, "/dashboard/clients/claude") || !strings.Contains(location, "flash=") {
		t.Errorf("expected redirect to client detail with flash message, got Location: %s", location)
	}

	// Verify settings.json carries the suffixed model verbatim, and no
	// CLAUDE_CODE_MAX_CONTEXT_TOKENS is written (the suffix replaces it).
	claudeSettings := filepath.Join(tempDir, ".claude", "settings.json")
	data, err := os.ReadFile(claudeSettings)
	if err != nil {
		t.Fatalf("failed to read claude settings: %v", err)
	}
	if !strings.Contains(string(data), "ANTHROPIC_DEFAULT_OPUS_MODEL") || !strings.Contains(string(data), "anthropic:claude-sonnet-4-5[1m]") {
		t.Errorf("expected ANTHROPIC_DEFAULT_OPUS_MODEL with [1m] suffix in settings.json, got: %s", string(data))
	}
	if strings.Contains(string(data), "CLAUDE_CODE_MAX_CONTEXT_TOKENS") {
		t.Errorf("expected no CLAUDE_CODE_MAX_CONTEXT_TOKENS in settings.json, got: %s", string(data))
	}
}

func TestDashboardDialogAsset(t *testing.T) {
	mux, _, _ := setupTestDashboard(t)

	req := httptest.NewRequest("GET", "/dashboard/assets/dialog.js", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /dashboard/assets/dialog.js, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "javascript") {
		t.Errorf("expected javascript content-type, got: %s", contentType)
	}

	if len(rec.Body.String()) == 0 {
		t.Errorf("expected non-empty dialog.js body")
	}
}

func TestDashboardOpencodeReopenConfiguredModelSlot(t *testing.T) {
	mux, sessID, tempDir := setupTestDashboard(t)

	// 1. Write an opencode config with a configured model (e.g. custom-gpt-4o)
	opencodeDir := filepath.Join(tempDir, ".config", "opencode")
	_ = os.MkdirAll(opencodeDir, 0755)
	configContent := `{
		"provider": {
			"tinyroute": {
				"npm": "@ai-sdk/openai-compatible",
				"options": {
					"baseURL": "http://127.0.0.1:8787/openai/v1",
					"apiKey": "tr_live_testkey"
				},
				"models": {
					"custom-whitelisted-model": {
						"name": "custom-whitelisted-model"
					}
				}
			}
		},
		"model": "tinyroute/custom-whitelisted-model"
	}`
	if err := os.WriteFile(filepath.Join(opencodeDir, "opencode.json"), []byte(configContent), 0600); err != nil {
		t.Fatalf("failed to write opencode config: %v", err)
	}

	// 2. Reopen GET /dashboard/clients/opencode
	req := httptest.NewRequest("GET", "/dashboard/clients/opencode", nil)
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /dashboard/clients/opencode, got %d", rec.Code)
	}

	body := rec.Body.String()
	// Configured model should be shown on the Model slots label and value
	if !strings.Contains(body, "custom-whitelisted-model") {
		t.Errorf("expected configured model 'custom-whitelisted-model' to be shown in model slot, got:\n%s", body)
	}
}

func TestDashboardOpencodeApplyCustomSelectedModel(t *testing.T) {
	mux, sessID, tempDir := setupTestDashboard(t)

	// Apply OpenCode with selected model opencode-zen:big-pickle and subagent opencode-zen:small-pickle
	form := url.Values{}
	form.Set("base_url", "http://127.0.0.1:8787/openai")
	form.Set("key_strategy", "mint")
	form.Set("slot_model", "opencode-zen:big-pickle")
	form.Set("slot_subagent", "opencode-zen:small-pickle")

	req := httptest.NewRequest("POST", "/dashboard/clients/opencode/apply", strings.NewReader(form.Encode()))
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for apply, got %d", rec.Code)
	}

	opencodeJSONPath := filepath.Join(tempDir, ".config", "opencode", "opencode.json")
	data, err := os.ReadFile(opencodeJSONPath)
	if err != nil {
		t.Fatalf("failed to read opencode.json: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `"model": "tinyroute/opencode-zen:big-pickle"`) {
		t.Errorf("expected model tinyroute/opencode-zen:big-pickle in opencode.json, got:\n%s", content)
	}
	if !strings.Contains(content, `"model": "tinyroute/opencode-zen:small-pickle"`) {
		t.Errorf("expected subagent tinyroute/opencode-zen:small-pickle in opencode.json, got:\n%s", content)
	}
	if strings.Contains(content, "gpt-4o") {
		t.Errorf("expected no fallback gpt-4o in opencode.json, got:\n%s", content)
	}
}

func TestDashboardOpencodePreservesConfiguredKeyOnReuse(t *testing.T) {
	mux, sessID, tempDir := setupTestDashboard(t)

	// Setup keys.json with a known key
	keysPath := filepath.Join(tempDir, "keys.json")
	kf := auth.KeyFile{
		Keys: []auth.Key{
			{
				ID:     "k_testopencode",
				Name:   "client-opencode",
				Prefix: "abcd",
				Digest: "fe7b25a575994616ce1f14f88233a322c54b5b57018b674b5b2a7e6e6b8ecac5",
				Secret: "tr_live_abcd1234existingsecret",
			},
		},
	}
	_ = auth.WriteKeyFile(keysPath, kf)

	// Write an existing opencode config with that apiKey
	opencodeDir := filepath.Join(tempDir, ".config", "opencode")
	_ = os.MkdirAll(opencodeDir, 0755)
	opencodeJSONPath := filepath.Join(opencodeDir, "opencode.json")
	configContent := `{
		"provider": {
			"tinyroute": {
				"npm": "@ai-sdk/openai-compatible",
				"options": {
					"baseURL": "http://127.0.0.1:8787/openai/v1",
					"apiKey": "tr_live_abcd1234existingsecret"
				},
				"models": {
					"opencode-zen:big-pickle": {
						"name": "opencode-zen:big-pickle"
					}
				}
			}
		},
		"model": "tinyroute/opencode-zen:big-pickle"
	}`
	_ = os.WriteFile(opencodeJSONPath, []byte(configContent), 0600)

	// 1. GET /dashboard/clients/opencode should have the key selected
	req := httptest.NewRequest("GET", "/dashboard/clients/opencode", nil)
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `name="existing_key" value="k_testopencode"`) {
		t.Errorf("expected k_testopencode to be the selected existing_key value, body:\n%s", rec.Body.String())
	}

	// 2. POST /dashboard/clients/opencode/apply with strategy=reuse and NO apiKey supplied
	form := url.Values{}
	form.Set("base_url", "http://127.0.0.1:8787/openai")
	form.Set("key_strategy", "reuse")
	form.Set("slot_model", "opencode-zen:big-pickle")

	reqApply := httptest.NewRequest("POST", "/dashboard/clients/opencode/apply", strings.NewReader(form.Encode()))
	reqApply.Host = "127.0.0.1:8787"
	reqApply.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqApply.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	recApply := httptest.NewRecorder()
	mux.ServeHTTP(recApply, reqApply)

	if recApply.Code != http.StatusOK && recApply.Code != http.StatusSeeOther {
		t.Fatalf("expected status 200 or 303 for apply, got %d", recApply.Code)
	}

	// Verify that opencode.json still has the existing secret and is NOT empty
	data, err := os.ReadFile(opencodeJSONPath)
	if err != nil {
		t.Fatalf("failed to read opencode.json: %v", err)
	}
	if !strings.Contains(string(data), `"apiKey": "tr_live_abcd1234existingsecret"`) {
		t.Errorf("expected apiKey 'tr_live_abcd1234existingsecret' to be preserved, got:\n%s", string(data))
	}
}

func TestDashboardClientDetail_ModelPickerTabsRender(t *testing.T) {
	mux, sessID, tempDir := setupTestDashboard(t)

	topoContent := `
providers:
  mock-anthropic:
    dialect: anthropic
    base_url: https://api.anthropic.com
    models:
    - claude-3-5-sonnet-20241022
    - claude-3-5-haiku-20241022
combos:
- name: claude-fast
  members:
  - mock-anthropic:claude-3-5-sonnet-20241022
- name: claude-smart
  members:
  - mock-anthropic:claude-3-5-sonnet-20241022
`
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte(topoContent), 0600); err != nil {
		t.Fatalf("write topo: %v", err)
	}
	t.Setenv("TINYROUTE_CONFIG", configPath)

	// Write Claude settings with sonnet set to combo:claude-fast
	claudeDir := filepath.Join(tempDir, ".claude")
	_ = os.MkdirAll(claudeDir, 0755)
	_ = os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{
		"env": {
			"ANTHROPIC_BASE_URL": "http://127.0.0.1:8787/anthropic",
			"ANTHROPIC_DEFAULT_SONNET_MODEL": "combo:claude-fast"
		}
	}`), 0600)

	req := httptest.NewRequest("GET", "/dashboard/clients/claude", nil)
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /dashboard/clients/claude, got %d", rec.Code)
	}

	body := rec.Body.String()

	// 1. Tab triggers exist when combos exist
	if !strings.Contains(body, `data-tui-tabs-value="models"`) || !strings.Contains(body, `data-tui-tabs-value="combos"`) {
		t.Errorf("expected tab triggers for models and combos")
	}

	// 2. Disjoint filter groups
	if !strings.Contains(body, `data-filter-input="picker-sonnet-models"`) {
		t.Errorf("expected models search input with picker-sonnet-models")
	}
	if !strings.Contains(body, `data-filter-input="picker-sonnet-combos"`) {
		t.Errorf("expected combos search input with picker-sonnet-combos")
	}
	if !strings.Contains(body, `data-filter-item="picker-sonnet-models"`) {
		t.Errorf("expected models filter items for picker-sonnet-models")
	}
	if !strings.Contains(body, `data-filter-item="picker-sonnet-combos"`) {
		t.Errorf("expected combos filter items for picker-sonnet-combos")
	}

	// 3. Provider group wrapper has data-filter-item and groupSearchStrings data-filter-text
	if !strings.Contains(body, `data-filter-item="picker-sonnet-models" data-filter-text="mock-anthropic claude-3-5-haiku-20241022 mock-anthropic:claude-3-5-haiku-20241022 claude-3-5-sonnet-20241022 mock-anthropic:claude-3-5-sonnet-20241022"`) {
		t.Errorf("expected group wrapper data-filter-text with provider and model info, body contains: %s", body)
	}

	// 4. DefaultValue is combos for combo-selected slot sonnet
	if !strings.Contains(body, `data-tui-tabs-value="combos" data-tui-tabs-state="active"`) &&
		!strings.Contains(body, `data-tui-tabs-state="active" data-tui-tabs-value="combos"`) {
		t.Errorf("expected combos tab trigger to be active for slot with combo value")
	}

	// 5. Check indicator on selected combo row for slot sonnet, and omitted on unselected combo row
	sonnetFastPattern := `data-slot-id="sonnet" data-model-val="combo:claude-fast"`
	fastIdx := strings.Index(body, sonnetFastPattern)
	if fastIdx == -1 {
		t.Fatalf("expected sonnet slot combo:claude-fast button")
	}
	btnSubstr := body[fastIdx:]
	if endBtn := strings.Index(btnSubstr, "</button>"); endBtn != -1 {
		btnHTML := btnSubstr[:endBtn]
		if !strings.Contains(btnHTML, "text-primary") || !strings.Contains(btnHTML, "<svg") {
			t.Errorf("expected check indicator in selected combo button for sonnet, got HTML: %s", btnHTML)
		}
	}
	sonnetSmartPattern := `data-slot-id="sonnet" data-model-val="combo:claude-smart"`
	smartIdx := strings.Index(body, sonnetSmartPattern)
	if smartIdx != -1 {
		btnSubstr := body[smartIdx:]
		if endBtn := strings.Index(btnSubstr, "</button>"); endBtn != -1 {
			btnHTML := btnSubstr[:endBtn]
			if strings.Contains(btnHTML, "text-primary") || strings.Contains(btnHTML, "<svg") {
				t.Errorf("expected NO check indicator in unselected combo button for sonnet, got HTML: %s", btnHTML)
			}
		}
	}

	// 6. Clear row (None / Default) present in both panes for optional slots (e.g. opus / haiku)
	if !strings.Contains(body, `data-filter-item="picker-haiku-models"`) || !strings.Contains(body, `data-filter-item="picker-haiku-combos"`) {
		t.Errorf("expected clear row in both panes for optional slot")
	}

	// 7. No-combos scenario: Tab bar omitted when no combos configured
	topoNoCombosContent := `
providers:
  mock-anthropic:
    dialect: anthropic
    base_url: https://api.anthropic.com
    models:
    - claude-3-5-sonnet-20241022
`
	if err := os.WriteFile(configPath, []byte(topoNoCombosContent), 0600); err != nil {
		t.Fatalf("write topo: %v", err)
	}

	reqNoCombos := httptest.NewRequest("GET", "/dashboard/clients/claude", nil)
	reqNoCombos.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	recNoCombos := httptest.NewRecorder()

	mux.ServeHTTP(recNoCombos, reqNoCombos)
	if recNoCombos.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recNoCombos.Code)
	}
	bodyNoCombos := recNoCombos.Body.String()

	if strings.Contains(bodyNoCombos, `data-tui-tabs-trigger`) {
		t.Errorf("expected NO tab triggers when no combos are configured")
	}
	if !strings.Contains(bodyNoCombos, `data-filter-input="picker-sonnet-models"`) {
		t.Errorf("expected flat models pane with picker-sonnet-models search input")
	}
}
