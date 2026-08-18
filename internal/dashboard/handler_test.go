package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/auth"
	"github.com/oniharnantyo/tinyroute/internal/config"
	_ "github.com/oniharnantyo/tinyroute/internal/dialect/anthropic"
	_ "github.com/oniharnantyo/tinyroute/internal/dialect/gemini"
	_ "github.com/oniharnantyo/tinyroute/internal/dialect/openai"
	_ "github.com/oniharnantyo/tinyroute/internal/dialect/openairesponses"
	"github.com/oniharnantyo/tinyroute/internal/history"
	"github.com/oniharnantyo/tinyroute/internal/route"
)

type mockHistoryQuerier struct {
	lastFilter history.Filter
	nextCursor string
}

func (m *mockHistoryQuerier) List(ctx context.Context, filter history.Filter) ([]history.Summary, string, error) {
	m.lastFilter = filter
	return []history.Summary{
		{
			ID:           "req-123",
			Timestamp:    time.Now(),
			Provider:     "openai",
			ModelReq:     "gpt-4o",
			Outcome:      "ok",
			InputTokens:  100,
			OutputTokens: 50,
			Latency:      200 * time.Millisecond,
			Attempts:     `[{"provider":"openai","model":"gpt-4o","status":200,"elapsed_ms":200}]`,
		},
	}, m.nextCursor, nil
}

func (m *mockHistoryQuerier) Get(ctx context.Context, id string) (history.Summary, bool, error) {
	if id == "req-123" {
		return history.Summary{
			ID:                    "req-123",
			Timestamp:             time.Now(),
			Provider:              "openai",
			ModelReq:              "gpt-4o",
			Outcome:               "ok",
			InputTokens:           100,
			OutputTokens:          50,
			Latency:               200 * time.Millisecond,
			RequestBody:           `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`,
			ResponseBody:          `{"choices":[{"message":{"role":"assistant","content":"world"}}]}`,
			TranslatedRequestBody: `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`,
			RawResponseBody:       `{"choices":[{"message":{"role":"assistant","content":"world"}}]}`,
			Attempts:              `[{"provider":"openai","model":"gpt-4o","status":200,"elapsed_ms":200}]`,
		}, true, nil
	}
	return history.Summary{}, false, nil
}

func (m *mockHistoryQuerier) LastUseByKey(ctx context.Context) (map[string]time.Time, error) {
	return map[string]time.Time{}, nil
}

func setupTestMux(t *testing.T) (*http.ServeMux, *Deps, string) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	keysPath := filepath.Join(tmpDir, "keys.json")
	passPath := filepath.Join(tmpDir, "dashboard.json")

	initialConfig := `providers:
  openai:
    dialect: openai
    base_url: https://api.openai.com/v1
    api_key: sk-test
    models:
    - gpt-4o
routes:
- from: openai
  match: '*'
  chain:
  - openai:gpt-4o`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	initialKeys := `{"keys": [{"id": "k_1", "prefix": "test", "digest": "abc"}]}`
	if err := os.WriteFile(keysPath, []byte(initialKeys), 0600); err != nil {
		t.Fatalf("failed to write keys: %v", err)
	}

	topoWatcher, err := config.NewWatcher(configPath, config.ParseTopology)
	if err != nil {
		t.Fatalf("failed to create topo watcher: %v", err)
	}

	keyWatcher, err := config.NewWatcher(keysPath, auth.ParseKeyFile)
	if err != nil {
		t.Fatalf("failed to create key watcher: %v", err)
	}

	passStore, err := NewPasswordStore(passPath)
	if err != nil {
		t.Fatalf("failed to create password store: %v", err)
	}

	sessStore := NewSessionStore()
	loginLimiter := NewLoginLimiter()
	healthStore := route.NewHealthStore(route.RealClock{}, filepath.Join(tmpDir, "state.json"))

	deps := &Deps{
		Service: config.Service{
			ConfigPath:            configPath,
			KeysPath:              keysPath,
			DashboardPasswordPath: passPath,
		},
		PasswordStore:   passStore,
		SessionStore:    sessStore,
		LoginLimiter:    loginLimiter,
		TopologyWatcher: topoWatcher,
		KeyWatcher:      keyWatcher,
		HealthStore:     healthStore,
		HistoryQuerier:  &mockHistoryQuerier{},
	}

	mux := http.NewServeMux()
	RegisterRoutes(mux, deps)
	return mux, deps, tmpDir
}

func TestUnauthenticatedRedirect(t *testing.T) {
	mux, _, _ := setupTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/overview", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status 303 redirect for unauthenticated access, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/dashboard/login" {
		t.Errorf("expected redirect to /dashboard/login, got %s", loc)
	}
}

func TestLoginViewRendering(t *testing.T) {
	mux, _, _ := setupTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/login", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK for /dashboard/login, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "tinyroute dashboard") || !strings.Contains(body, "Sign In") {
		t.Errorf("expected complete login page HTML, got: %s", body)
	}
}

func TestLoginAndSessionFlow(t *testing.T) {
	mux, deps, _ := setupTestMux(t)

	// 1. Submit wrong password
	form := url.Values{}
	form.Set("password", "wrongpass")
	req := httptest.NewRequest(http.MethodPost, "/dashboard/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect on login failure, got %d", rec.Code)
	}

	// 2. Submit default password
	form.Set("password", DefaultPassword)
	req = httptest.NewRequest(http.MethodPost, "/dashboard/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	rec = httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect on successful login, got %d", rec.Code)
	}

	cookies := rec.Result().Cookies()
	var sessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			sessCookie = c
			break
		}
	}
	if sessCookie == nil {
		t.Fatalf("expected session cookie to be set")
	}

	// SameSite=Lax is required so the session cookie survives the cross-site
	// redirect from an OAuth provider (e.g. Antigravity PKCE flow) back to
	// /dashboard/oauth/callback. Strict would be withheld by the browser and
	// the callback would bounce to /dashboard/login.
	if sessCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSite=Lax on session cookie, got %v", sessCookie.SameSite)
	}

	// 3. Access protected route with session cookie
	req = httptest.NewRequest(http.MethodGet, "/dashboard/overview", nil)
	req.AddCookie(sessCookie)
	rec = httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 OK for authenticated overview page, got %d", rec.Code)
	}

	// 4. Access all views
	for _, path := range []string{"/dashboard/providers", "/dashboard/routes", "/dashboard/history", "/dashboard/keys", "/dashboard/settings"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(sessCookie)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 OK for %s, got %d", path, rec.Code)
		}
	}

	// 5. Logout
	req = httptest.NewRequest(http.MethodGet, "/dashboard/logout", nil)
	req.AddCookie(sessCookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if !deps.SessionStore.ValidateSession(sessCookie.Value) {
		// Session revoked as expected
	} else {
		t.Errorf("expected session to be revoked after logout")
	}
}

func TestProviderAndModelMutations(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// Add Model
	form := url.Values{}
	form.Set("provider", "openai")
	form.Set("model", "gpt-4o-mini")
	req := httptest.NewRequest(http.MethodPost, "/dashboard/models/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect after model add, got %d", rec.Code)
	}

	// Verify model was added to config.yaml
	data, _ := os.ReadFile(deps.Service.ConfigPath)
	topo, _ := config.ParseRawTopology(data)
	models := topo.Providers["openai"].Models
	found := false
	for _, m := range models {
		if m == "gpt-4o-mini" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'gpt-4o-mini' to be added to openai provider models")
	}

	// Remove Model
	form = url.Values{}
	form.Set("provider", "openai")
	form.Set("model", "gpt-4o-mini")
	req = httptest.NewRequest(http.MethodPost, "/dashboard/models/remove", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect after model remove, got %d", rec.Code)
	}
}

func TestPasswordChangeHandler(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	form := url.Values{}
	form.Set("current_password", DefaultPassword)
	form.Set("new_password", "newsecret999")
	form.Set("confirm_password", "newsecret999")

	req := httptest.NewRequest(http.MethodPost, "/dashboard/settings/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect after password change, got %d", rec.Code)
	}

	if !deps.PasswordStore.VerifyPassword("newsecret999") {
		t.Errorf("expected new password 'newsecret999' to be active")
	}
}

func TestProviderCRUDAndCredentials(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// 1. Add Provider via Preset
	form := url.Values{}
	form.Set("preset_name", "groq")
	req := httptest.NewRequest(http.MethodPost, "/dashboard/providers/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect after provider add, got %d", rec.Code)
	}

	// Verify groq provider added
	data, _ := os.ReadFile(deps.Service.ConfigPath)
	topo, _ := config.ParseRawTopology(data)
	if _, ok := topo.Providers["groq"]; !ok {
		t.Errorf("expected 'groq' provider in topology")
	}

	// 2. Set Provider Credential
	form = url.Values{}
	form.Set("name", "groq")
	form.Set("api_key", "gsk_test123456789")
	req = httptest.NewRequest(http.MethodPost, "/dashboard/providers/credential", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect after setting provider credential, got %d", rec.Code)
	}

	// 3. Delete Provider
	form = url.Values{}
	form.Set("name", "groq")
	req = httptest.NewRequest(http.MethodPost, "/dashboard/providers/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect after deleting provider, got %d", rec.Code)
	}

	data, _ = os.ReadFile(deps.Service.ConfigPath)
	topo, _ = config.ParseRawTopology(data)
	if _, ok := topo.Providers["groq"]; ok {
		t.Errorf("expected 'groq' provider to be removed")
	}
}

func TestModelTestHandler(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	// The model test now probes in-process through the gateway. Inject a fake
	// RunProbe so we assert the handler wiring (form → RunProbe → flash redirect)
	// without standing up a live listener or upstream.
	deps.RunProbe = func(ctx context.Context, provName, dialectName, model string, timeout time.Duration) (int, time.Duration, error) {
		if provName != "openai" || model != "gpt-4o" {
			return 0, 0, fmt.Errorf("unexpected probe args %s/%s/%s", provName, dialectName, model)
		}
		return http.StatusOK, 1 * time.Millisecond, nil
	}
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// Trigger model test (Form redirect)
	form := url.Values{}
	form.Set("provider", "openai")
	form.Set("model", "gpt-4o")
	req := httptest.NewRequest(http.MethodPost, "/dashboard/models/test", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect after model test, got %d", rec.Code)
	}

	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "flash=") {
		t.Errorf("expected flash message in redirect URL after successful model test, got %s", loc)
	}

	// Trigger model test via JSON (Approach 1: Fetch API with no redirect)
	reqJSON := httptest.NewRequest(http.MethodPost, "/dashboard/models/test", strings.NewReader(form.Encode()))
	reqJSON.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqJSON.Header.Set("Accept", "application/json")
	reqJSON.Host = "127.0.0.1:8787"
	reqJSON.AddCookie(cookie)
	recJSON := httptest.NewRecorder()

	mux.ServeHTTP(recJSON, reqJSON)
	if recJSON.Code != http.StatusOK {
		t.Errorf("expected 200 OK for JSON probe request, got %d", recJSON.Code)
	}
	if !strings.Contains(recJSON.Body.String(), `"ok":true`) {
		t.Errorf("expected ok:true in JSON response, got %s", recJSON.Body.String())
	}
}

func TestHandlerEdgeCases(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// 1. Password change mismatch
	form := url.Values{}
	form.Set("current_password", DefaultPassword)
	form.Set("new_password", "passA123")
	form.Set("confirm_password", "passB123")
	req := httptest.NewRequest(http.MethodPost, "/dashboard/settings/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error redirect on password mismatch")
	}

	// 2. Empty model add
	form = url.Values{}
	form.Set("provider", "openai")
	form.Set("model", "")
	req = httptest.NewRequest(http.MethodPost, "/dashboard/models/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error redirect on empty model name")
	}

	// 3. History view with filter
	req = httptest.NewRequest(http.MethodGet, "/dashboard/history?key=k_1&model=gpt-4o", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for history view with filter, got %d", rec.Code)
	}
}

func TestHandlerMoreErrorBranchesAndViews(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// 1. GET /dashboard/login with error string
	req := httptest.NewRequest(http.MethodGet, "/dashboard/login?error=TestError", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for login view, got %d", rec.Code)
	}

	// 2. Add provider with unknown preset
	form := url.Values{}
	form.Set("preset_name", "nonexistent_preset_xyz")
	req = httptest.NewRequest(http.MethodPost, "/dashboard/providers/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error redirect on unknown preset add")
	}

	// 3. Provider credential with empty API key
	form = url.Values{}
	form.Set("name", "openai")
	form.Set("api_key", "")
	req = httptest.NewRequest(http.MethodPost, "/dashboard/providers/credential", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error redirect on empty API key")
	}

	// 4. Model test for non-existent provider
	form = url.Values{}
	form.Set("provider", "nonexistent_provider")
	form.Set("model", "gpt-4o")
	req = httptest.NewRequest(http.MethodPost, "/dashboard/models/test", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error redirect on model test for nonexistent provider")
	}

	// 5. Password change with wrong current password
	form = url.Values{}
	form.Set("current_password", "wrongpass")
	form.Set("new_password", "newsecret123")
	form.Set("confirm_password", "newsecret123")
	req = httptest.NewRequest(http.MethodPost, "/dashboard/settings/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error redirect on wrong current password")
	}

	// 6. Password change with short new password
	form = url.Values{}
	form.Set("current_password", DefaultPassword)
	form.Set("new_password", "123")
	form.Set("confirm_password", "123")
	req = httptest.NewRequest(http.MethodPost, "/dashboard/settings/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error redirect on short new password")
	}
}

func TestProviderDetailViewAndCustomAdd(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// 1. GET Provider Detail view for existing provider "openai"
	req := httptest.NewRequest(http.MethodGet, "/dashboard/providers/openai", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for /dashboard/providers/openai, got %d", rec.Code)
	}

	// 2. GET Provider Detail view for non-existent provider
	req = httptest.NewRequest(http.MethodGet, "/dashboard/providers/nonexistent_xyz", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect for non-existent provider detail view, got %d", rec.Code)
	}

	// 3. POST Add Custom Provider
	form := url.Values{}
	form.Set("name", "custom-llm")
	form.Set("dialect", "openai")
	form.Set("base_url", "https://api.custom.com/v1")

	req = httptest.NewRequest(http.MethodPost, "/dashboard/providers/add-custom", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect after adding custom provider, got %d", rec.Code)
	}

	if !strings.Contains(rec.Header().Get("Location"), "/dashboard/providers/custom-llm") {
		t.Errorf("expected redirect to custom-llm detail page, got %s", rec.Header().Get("Location"))
	}

	// Verify custom provider added to topology
	data, _ := os.ReadFile(deps.Service.ConfigPath)
	topo, _ := config.ParseRawTopology(data)
	if p, ok := topo.Providers["custom-llm"]; !ok || p.BaseURL != "https://api.custom.com/v1" {
		t.Errorf("expected custom-llm provider in topology")
	}
}

func TestProvidersCatalogViewAndDedup(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// 1. Add custom provider to config
	form := url.Values{}
	form.Set("name", "my-custom-provider")
	form.Set("dialect", "openai")
	form.Set("base_url", "https://api.custom.com/v1")
	req := httptest.NewRequest(http.MethodPost, "/dashboard/providers/add-custom", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// 2. GET /dashboard/providers
	req = httptest.NewRequest(http.MethodGet, "/dashboard/providers", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /dashboard/providers, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Free Tier") {
		t.Errorf("expected Free Tier section header in body")
	}
	if !strings.Contains(body, "API Key") {
		t.Errorf("expected API Key section header in body")
	}
	if !strings.Contains(body, "my-custom-provider") {
		t.Errorf("expected custom provider to render in providers view")
	}

	// 3. GET /dashboard/providers/openai detail view
	req = httptest.NewRequest(http.MethodGet, "/dashboard/providers/openai", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /dashboard/providers/openai, got %d", rec.Code)
	}

	bodyDetail := rec.Body.String()
	if !strings.Contains(bodyDetail, "Models (") {
		t.Errorf("expected Models section in detail view")
	}

	// 4. Test Model Add from detail view redirects back to /dashboard/providers/openai
	form = url.Values{}
	form.Set("provider", "openai")
	form.Set("model", "gpt-4o-mini")
	req = httptest.NewRequest(http.MethodPost, "/dashboard/models/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect for model add, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/dashboard/providers/openai") {
		t.Errorf("expected redirect to stay on detail view /dashboard/providers/openai, got %s", loc)
	}
}

func TestProvidersSectionBucketingAndModelDedup(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// Add custom provider
	form := url.Values{}
	form.Set("name", "custom-api-provider")
	form.Set("dialect", "openai")
	form.Set("base_url", "https://api.custom-api.com/v1")
	req := httptest.NewRequest(http.MethodPost, "/dashboard/providers/add-custom", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// GET /dashboard/providers
	req = httptest.NewRequest(http.MethodGet, "/dashboard/providers", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /dashboard/providers, got %d", rec.Code)
	}
	body := rec.Body.String()

	// 6.1: Assert Free Tier priority pull and API Key sections render, while empty OAuth section is skipped per D2/Task 2.2
	if !strings.Contains(body, "Free Tier") {
		t.Errorf("expected 'Free Tier' section to be rendered")
	}
	if !strings.Contains(body, "API Key") {
		t.Errorf("expected 'API Key' section to be rendered")
	}
	if strings.Contains(body, "OAuth (") {
		t.Errorf("expected empty 'OAuth' section to be skipped")
	}

	// 6.2: Assert custom provider renders under API Key section with Manage link
	if !strings.Contains(body, "custom-api-provider") {
		t.Errorf("expected custom-api-provider to render in providers view")
	}

	// 6.3: Assert handleProviderDetailView excludes already whitelisted models
	req = httptest.NewRequest(http.MethodGet, "/dashboard/providers/openai", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for provider detail view, got %d", rec.Code)
	}
	detailBody := rec.Body.String()

	// Models section contains gpt-4o
	if !strings.Contains(detailBody, "gpt-4o") {
		t.Errorf("expected model gpt-4o in detail view")
	}
}

func TestDashboardOAuthRoutes(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// 1. Test OAuth Start for PKCE provider (claude)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/providers/claude/oauth/start", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect for PKCE OAuth start, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "claude.ai/oauth/authorize") {
		t.Errorf("expected authorize URL redirect, got %s", loc)
	}

	// 2. Test OAuth Start for non-OAuth provider (custom or unknown)
	req = httptest.NewRequest(http.MethodGet, "/dashboard/providers/unknown-prov/oauth/start", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect for non-OAuth provider, got %d", rec.Code)
	}

	// 3. Test Device Poll endpoint
	form := url.Values{}
	form.Set("device_code", "test_code")
	req = httptest.NewRequest(http.MethodPost, "/dashboard/providers/xai/oauth/device/poll", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for device poll endpoint, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status"`) {
		t.Errorf("expected status JSON response, got %s", rec.Body.String())
	}

	// 4. Callback missing state parameter
	req = httptest.NewRequest(http.MethodGet, "/dashboard/oauth/callback", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=Missing+OAuth+state") {
		t.Errorf("expected missing state error redirect, got code %d loc %s", rec.Code, rec.Header().Get("Location"))
	}

	// 5. Callback with unknown state parameter
	req = httptest.NewRequest(http.MethodGet, "/dashboard/oauth/callback?state=bogus_state", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=Invalid+or+expired+OAuth+state") {
		t.Errorf("expected invalid state error redirect, got code %d loc %s", rec.Code, rec.Header().Get("Location"))
	}

	// 6. Callback with valid state but error response from provider
	req = httptest.NewRequest(http.MethodGet, "/dashboard/providers/claude/oauth/start", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	loc = rec.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse authorize URL failed: %v", err)
	}
	validState := u.Query().Get("state")
	if validState == "" {
		t.Fatalf("expected state in authorize URL %s", loc)
	}

	// First callback with error param
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/dashboard/oauth/callback?state=%s&error=access_denied", validState), nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "access_denied") {
		t.Errorf("expected access_denied redirect to detail view, got loc %s", rec.Header().Get("Location"))
	}

	// Reuse same state -> single use validation check (must fail as invalid/expired)
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/dashboard/oauth/callback?state=%s&code=some_code", validState), nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=Invalid+or+expired+OAuth+state") {
		t.Errorf("expected state reuse to fail with invalid state error, got loc %s", rec.Header().Get("Location"))
	}
}

func TestProviderCredentialDelete(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// 1. Delete API Key
	form := url.Values{}
	form.Set("name", "openai")
	form.Set("type", "api_key")
	req := httptest.NewRequest(http.MethodPost, "/dashboard/providers/credential/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect for API key delete, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "API+key+removed") {
		t.Errorf("expected flash API key removed, got %s", rec.Header().Get("Location"))
	}

	// 2. Delete OAuth credential
	form = url.Values{}
	form.Set("name", "openai")
	form.Set("type", "oauth")
	form.Set("account", "default")
	req = httptest.NewRequest(http.MethodPost, "/dashboard/providers/credential/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect for OAuth credential delete, got %d", rec.Code)
	}
}

func TestEnsureMaterializedHelper(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`{"providers":{}}`), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	data, _ := os.ReadFile(configPath)
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		t.Fatalf("failed to parse topo: %v", err)
	}

	// 1. Materialize unconfigured preset (e.g. groq)
	mat, err := ensureMaterialized(configPath, &rawTopo, "groq")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mat {
		t.Errorf("expected groq to be materialized")
	}
	if p, ok := rawTopo.Providers["groq"]; !ok || p.APIKey == "" {
		t.Errorf("expected groq in topology with credential var placeholder, got %+v", p)
	}

	// 2. Idempotent call on already present provider
	mat, err = ensureMaterialized(configPath, &rawTopo, "groq")
	if err != nil || mat {
		t.Errorf("expected second call to be no-op (mat=false), got mat=%v err=%v", mat, err)
	}

	// 3. Unknown non-preset provider returns false, nil
	mat, err = ensureMaterialized(configPath, &rawTopo, "nonexistent-provider-xyz")
	if err != nil || mat {
		t.Errorf("expected non-preset to return false, nil; got mat=%v err=%v", mat, err)
	}
}

func TestLazyMaterializationMutations(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// 1. Save static API key on unconfigured preset (groq)
	form := url.Values{}
	form.Set("name", "groq")
	form.Set("api_key", "gsk_lazy_test_key")
	req := httptest.NewRequest(http.MethodPost, "/dashboard/providers/credential", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after setting key on unconfigured preset, got %d", rec.Code)
	}
	if strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected success redirect, got error: %s", rec.Header().Get("Location"))
	}

	// Verify groq was materialized with the new APIKey
	data, _ := os.ReadFile(deps.Service.ConfigPath)
	topo, _ := config.ParseRawTopology(data)
	if p, ok := topo.Providers["groq"]; !ok || p.APIKey != "gsk_lazy_test_key" {
		t.Errorf("expected groq materialized with api_key 'gsk_lazy_test_key', got %+v", p)
	}

	// 2. Whitelist model on unconfigured preset (anthropic)
	form = url.Values{}
	form.Set("provider", "anthropic")
	form.Set("model", "claude-3-5-sonnet-20241022")
	req = httptest.NewRequest(http.MethodPost, "/dashboard/models/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after model add on unconfigured preset, got %d", rec.Code)
	}
	if strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected success redirect, got error: %s", rec.Header().Get("Location"))
	}

	// Verify anthropic was materialized with the whitelisted model
	data, _ = os.ReadFile(deps.Service.ConfigPath)
	topo, _ = config.ParseRawTopology(data)
	if p, ok := topo.Providers["anthropic"]; !ok || len(p.Models) == 0 || p.Models[0] != "claude-3-5-sonnet-20241022" {
		t.Errorf("expected anthropic materialized with whitelisted model, got %+v", p)
	}

	// 3. Mutation on genuinely unknown provider name rejected
	form = url.Values{}
	form.Set("name", "unknown-bogus-provider")
	form.Set("api_key", "sk-123")
	req = httptest.NewRequest(http.MethodPost, "/dashboard/providers/credential", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	if !strings.Contains(rec.Header().Get("Location"), "Provider+not+found") {
		t.Errorf("expected 'Provider not found' error redirect for unknown provider, got %s", rec.Header().Get("Location"))
	}
}

func TestDeletePresetRevert(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// Delete configured preset-backed provider (openai)
	form := url.Values{}
	form.Set("name", "openai")
	req := httptest.NewRequest(http.MethodPost, "/dashboard/providers/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after delete, got %d", rec.Code)
	}

	// Verify openai is removed from topology
	data, _ := os.ReadFile(deps.Service.ConfigPath)
	topo, _ := config.ParseRawTopology(data)
	if _, ok := topo.Providers["openai"]; ok {
		t.Errorf("expected 'openai' to be deleted from topology")
	}
}

func TestAvailablePresetDetailView(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// View detail page for available unconfigured preset (groq)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/providers/groq", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for available preset detail view, got %d", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, "Provider Not Configured") {
		t.Errorf("expected encouragement banner to be removed")
	}
	if !strings.Contains(body, "Models") {
		t.Errorf("expected Models section to render for available preset")
	}
}

func TestHistoryView_FilteringAndPagination(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	mockHQ := deps.HistoryQuerier.(*mockHistoryQuerier)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// 1. Check filter parsing with dates, provider, limit
	req := httptest.NewRequest(http.MethodGet, "/dashboard/history?provider=openai&from=2026-08-01&to=2026-08-05&limit=25", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for history view, got %d", rec.Code)
	}

	if mockHQ.lastFilter.Provider != "openai" {
		t.Errorf("expected filter Provider 'openai', got %q", mockHQ.lastFilter.Provider)
	}
	if mockHQ.lastFilter.Limit != 25 {
		t.Errorf("expected filter Limit 25, got %d", mockHQ.lastFilter.Limit)
	}

	expectedFrom := time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)
	if !mockHQ.lastFilter.From.Equal(expectedFrom) {
		t.Errorf("expected filter From %v, got %v", expectedFrom, mockHQ.lastFilter.From)
	}

	expectedTo := time.Date(2026, 8, 5, 23, 59, 59, 999999999, time.Local)
	if !mockHQ.lastFilter.To.Equal(expectedTo) {
		t.Errorf("expected filter To %v, got %v", expectedTo, mockHQ.lastFilter.To)
	}

	// 2. Clamping of limit above MaxListLimit (500)
	req = httptest.NewRequest(http.MethodGet, "/dashboard/history?limit=1000", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if mockHQ.lastFilter.Limit != 500 {
		t.Errorf("expected clamped Limit 500, got %d", mockHQ.lastFilter.Limit)
	}

	// 3. Load More link rendered when nextCursor exists, preserving all filters and growing limit
	mockHQ.nextCursor = "cursor-xyz"
	req = httptest.NewRequest(http.MethodGet, "/dashboard/history?provider=openai&from=2026-08-01&limit=50", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "Load More") {
		t.Errorf("expected Load More button when HasMore is true")
	}
	if !strings.Contains(body, "limit=100") {
		t.Errorf("expected Load More link to increment limit to 100, got body: %s", body)
	}
	if !strings.Contains(body, "provider=openai") || !strings.Contains(body, "from=2026-08-01") {
		t.Errorf("expected Load More link to preserve active filters, got body: %s", body)
	}
	if !strings.Contains(body, "#history-table") {
		t.Errorf("expected Load More link to have #history-table anchor fragment")
	}

	// 4. Status badge code rendered directly (not hardcoded 200 OK / 500 Error)
	if !strings.Contains(body, "200") {
		t.Errorf("expected status code 200 rendered in status badge")
	}
}

func TestHistoryDetailView(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// 1. Unauthenticated request redirects to login
	unauthReq := httptest.NewRequest(http.MethodGet, "/dashboard/history/req-123", nil)
	unauthRec := httptest.NewRecorder()
	mux.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusSeeOther || !strings.Contains(unauthRec.Header().Get("Location"), "/dashboard/login") {
		t.Errorf("expected redirect to login for unauthenticated detail view, got status %d, loc: %s", unauthRec.Code, unauthRec.Header().Get("Location"))
	}

	// 2. Existing request ID returns 200 and renders details and bodies
	req := httptest.NewRequest(http.MethodGet, "/dashboard/history/req-123", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for history detail view, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "req-123") {
		t.Errorf("expected request ID req-123 in body")
	}
	if !strings.Contains(body, "Client Request") || !strings.Contains(body, "Provider Request") || !strings.Contains(body, "Provider Response") || !strings.Contains(body, "Final Response") {
		t.Errorf("expected all 4 body panes in history detail view")
	}
	if !strings.Contains(body, "Attempt Chain") {
		t.Errorf("expected Attempt Chain in history detail view")
	}
	if !strings.Contains(body, "/dashboard/history") {
		t.Errorf("expected back link to history")
	}

	// 3. Unknown request ID renders 200 with Not Found state
	req404 := httptest.NewRequest(http.MethodGet, "/dashboard/history/nonexistent-req", nil)
	req404.AddCookie(cookie)
	rec404 := httptest.NewRecorder()
	mux.ServeHTTP(rec404, req404)

	if rec404.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for not-found history detail view, got %d", rec404.Code)
	}
	body404 := rec404.Body.String()
	if !strings.Contains(body404, "Request not found") {
		t.Errorf("expected 'Request not found' in 404 state body")
	}
}

func TestKeysManagementLifecycle(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	// 1. Unauthenticated requests redirect to login
	for _, path := range []string{
		"/dashboard/keys",
		"/dashboard/keys/create",
		"/dashboard/keys/k_1/update",
		"/dashboard/keys/k_1/revoke",
	} {
		method := http.MethodGet
		if strings.Contains(path, "create") || strings.Contains(path, "update") || strings.Contains(path, "revoke") {
			method = http.MethodPost
		}
		unauthReq := httptest.NewRequest(method, path, nil)
		unauthReq.Host = "127.0.0.1:8787"
		unauthRec := httptest.NewRecorder()
		mux.ServeHTTP(unauthRec, unauthReq)
		if unauthRec.Code != http.StatusSeeOther || !strings.Contains(unauthRec.Header().Get("Location"), "/dashboard/login") {
			t.Errorf("expected redirect to login for unauthenticated %s %s, got status %d, loc: %s", method, path, unauthRec.Code, unauthRec.Header().Get("Location"))
		}
	}

	// 2. Create key with name, 7d expiry, rate limit
	createForm := url.Values{
		"name":          {"laptop-agent"},
		"expiry_choice": {"7d"},
		"rate_requests": {"60"},
		"rate_interval": {"1m"},
	}
	createReq := httptest.NewRequest(http.MethodPost, "/dashboard/keys/create", strings.NewReader(createForm.Encode()))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createReq.Host = "127.0.0.1:8787"
	createReq.AddCookie(cookie)
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on key create, got %d", createRec.Code)
	}
	loc := createRec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/dashboard/keys") || strings.Contains(loc, "error=") {
		t.Fatalf("unexpected create redirect location: %s", loc)
	}

	// Allow config watcher to catch file update
	time.Sleep(50 * time.Millisecond)

	// 3. View /dashboard/keys
	viewReq := httptest.NewRequest(http.MethodGet, "/dashboard/keys", nil)
	viewReq.Host = "127.0.0.1:8787"
	viewReq.AddCookie(cookie)
	viewRec := httptest.NewRecorder()
	mux.ServeHTTP(viewRec, viewReq)

	if viewRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on keys view, got %d", viewRec.Code)
	}
	viewBody := viewRec.Body.String()
	if !strings.Contains(viewBody, "laptop-agent") {
		t.Errorf("expected keys view to contain created key 'laptop-agent'")
	}
	if !strings.Contains(viewBody, "60 / 1m") {
		t.Errorf("expected rate '60 / 1m' in view body")
	}
	if !strings.Contains(viewBody, "active") {
		t.Errorf("expected 'active' status badge in view body")
	}

	// Get created key ID from key watcher
	ks := deps.KeyWatcher.Get()
	var createdKey auth.Key
	for _, k := range ks.Keys() {
		if k.Name == "laptop-agent" {
			createdKey = k
			break
		}
	}
	if createdKey.ID == "" {
		t.Fatalf("created key not found in key watcher")
	}

	// 4. Update key name and rate
	updateForm := url.Values{
		"name":          {"desktop-agent"},
		"expiry_choice": {"keep"},
		"rate_requests": {"120"},
		"rate_interval": {"1m"},
	}
	updateReq := httptest.NewRequest(http.MethodPost, "/dashboard/keys/"+createdKey.ID+"/update", strings.NewReader(updateForm.Encode()))
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateReq.Host = "127.0.0.1:8787"
	updateReq.AddCookie(cookie)
	updateRec := httptest.NewRecorder()
	mux.ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on key update, got %d", updateRec.Code)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify update
	viewReq2 := httptest.NewRequest(http.MethodGet, "/dashboard/keys", nil)
	viewReq2.Host = "127.0.0.1:8787"
	viewReq2.AddCookie(cookie)
	viewRec2 := httptest.NewRecorder()
	mux.ServeHTTP(viewRec2, viewReq2)
	viewBody2 := viewRec2.Body.String()
	if !strings.Contains(viewBody2, "desktop-agent") {
		t.Errorf("expected keys view to contain updated name 'desktop-agent'")
	}
	if !strings.Contains(viewBody2, "120 / 1m") {
		t.Errorf("expected rate '120 / 1m' in view body")
	}

	// 5. Revoke key
	revokeReq := httptest.NewRequest(http.MethodPost, "/dashboard/keys/"+createdKey.ID+"/revoke", nil)
	revokeReq.Host = "127.0.0.1:8787"
	revokeReq.AddCookie(cookie)
	revokeRec := httptest.NewRecorder()
	mux.ServeHTTP(revokeRec, revokeReq)

	if revokeRec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on key revoke, got %d", revokeRec.Code)
	}

	time.Sleep(50 * time.Millisecond)

	// 6. Verify revoked key is absent from view and its secret is never in page data
	viewReq3 := httptest.NewRequest(http.MethodGet, "/dashboard/keys", nil)
	viewReq3.Host = "127.0.0.1:8787"
	viewReq3.AddCookie(cookie)
	viewRec3 := httptest.NewRecorder()
	mux.ServeHTTP(viewRec3, viewReq3)
	viewBody3 := viewRec3.Body.String()

	if strings.Contains(viewBody3, "desktop-agent") {
		t.Errorf("revoked key 'desktop-agent' must not render in keys table")
	}
	if createdKey.Secret != "" && strings.Contains(viewBody3, createdKey.Secret) {
		t.Errorf("revoked key secret must never appear in page data")
	}

	// 7. Malformed input tests
	badForm := url.Values{"name": {""}}
	badReq := httptest.NewRequest(http.MethodPost, "/dashboard/keys/create", strings.NewReader(badForm.Encode()))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badReq.Host = "127.0.0.1:8787"
	badReq.AddCookie(cookie)
	badRec := httptest.NewRecorder()
	mux.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusSeeOther || !strings.Contains(badRec.Header().Get("Location"), "error=") {
		t.Errorf("expected error redirect for empty key name, got status %d, loc %s", badRec.Code, badRec.Header().Get("Location"))
	}
}

func TestKeysManagementErrorPaths(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	token := deps.SessionStore.CreateSession(1 * time.Hour)
	cookie := &http.Cookie{Name: SessionCookieName, Value: token}

	sendPost := func(urlPath string, form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, urlPath, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Host = "127.0.0.1:8787"
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	// 1. Create with 30d expiry
	rec := sendPost("/dashboard/keys/create", url.Values{
		"name":          {"bot-30d"},
		"expiry_choice": {"30d"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("create 30d failed: %v", rec.Header().Get("Location"))
	}

	// 2. Create with custom RFC3339 and datetime-local
	futureLocal := time.Now().Add(48 * time.Hour).Format("2006-01-02T15:04")
	rec = sendPost("/dashboard/keys/create", url.Values{
		"name":          {"bot-custom-local"},
		"expiry_choice": {"custom"},
		"expiry_custom": {futureLocal},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("create custom datetime-local failed: %v", rec.Header().Get("Location"))
	}

	futureRFC := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	rec = sendPost("/dashboard/keys/create", url.Values{
		"name":          {"bot-custom-rfc"},
		"expiry_choice": {"custom"},
		"expiry_custom": {futureRFC},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("create custom RFC3339 failed: %v", rec.Header().Get("Location"))
	}

	// 3. Create with invalid custom date format
	rec = sendPost("/dashboard/keys/create", url.Values{
		"name":          {"bot-bad-date"},
		"expiry_choice": {"custom"},
		"expiry_custom": {"not-a-date"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error for invalid custom date")
	}

	// 4. Create with past custom date
	pastDate := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	rec = sendPost("/dashboard/keys/create", url.Values{
		"name":          {"bot-past-date"},
		"expiry_choice": {"custom"},
		"expiry_custom": {pastDate},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error for past custom date")
	}

	// 5. Create with invalid rate limit requests and interval
	rec = sendPost("/dashboard/keys/create", url.Values{
		"name":          {"bot-bad-rate-reqs"},
		"rate_requests": {"-5"},
		"rate_interval": {"1m"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error for negative rate requests")
	}

	rec = sendPost("/dashboard/keys/create", url.Values{
		"name":          {"bot-bad-rate-intv"},
		"rate_requests": {"60"},
		"rate_interval": {"invalid-dur"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error for invalid rate interval")
	}

	// Create a valid key for update/revoke testing
	rec = sendPost("/dashboard/keys/create", url.Values{
		"name":          {"target-key"},
		"expiry_choice": {"7d"},
		"rate_requests": {"10"},
		"rate_interval": {"1m"},
	})
	time.Sleep(50 * time.Millisecond)
	ks := deps.KeyWatcher.Get()
	var targetKey auth.Key
	for _, k := range ks.Keys() {
		if k.Name == "target-key" {
			targetKey = k
			break
		}
	}
	if targetKey.ID == "" {
		t.Fatalf("failed to find target key")
	}

	// 6. Update error paths
	// Missing name
	rec = sendPost("/dashboard/keys/"+targetKey.ID+"/update", url.Values{
		"name": {""},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error for empty name in update")
	}

	// Unknown key ID
	rec = sendPost("/dashboard/keys/k_unknown_id/update", url.Values{
		"name": {"new-name"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error for unknown key ID in update")
	}

	// Update expiry never / 7d / 30d / custom
	rec = sendPost("/dashboard/keys/"+targetKey.ID+"/update", url.Values{
		"name":          {"target-key"},
		"expiry_choice": {"never"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("update expiry never failed: %v", rec.Header().Get("Location"))
	}

	rec = sendPost("/dashboard/keys/"+targetKey.ID+"/update", url.Values{
		"name":          {"target-key"},
		"expiry_choice": {"7d"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("update expiry 7d failed: %v", rec.Header().Get("Location"))
	}

	rec = sendPost("/dashboard/keys/"+targetKey.ID+"/update", url.Values{
		"name":          {"target-key"},
		"expiry_choice": {"30d"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("update expiry 30d failed: %v", rec.Header().Get("Location"))
	}

	rec = sendPost("/dashboard/keys/"+targetKey.ID+"/update", url.Values{
		"name":          {"target-key"},
		"expiry_choice": {"custom"},
		"expiry_custom": {futureLocal},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("update expiry custom local failed: %v", rec.Header().Get("Location"))
	}

	rec = sendPost("/dashboard/keys/"+targetKey.ID+"/update", url.Values{
		"name":          {"target-key"},
		"expiry_choice": {"custom"},
		"expiry_custom": {futureRFC},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("update expiry custom RFC failed: %v", rec.Header().Get("Location"))
	}

	// Invalid update custom date
	rec = sendPost("/dashboard/keys/"+targetKey.ID+"/update", url.Values{
		"name":          {"target-key"},
		"expiry_choice": {"custom"},
		"expiry_custom": {"bad-date"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error for bad custom date on update")
	}

	rec = sendPost("/dashboard/keys/"+targetKey.ID+"/update", url.Values{
		"name":          {"target-key"},
		"expiry_choice": {"custom"},
		"expiry_custom": {pastDate},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error for past custom date on update")
	}

	// Clear rate & bad rate on update
	rec = sendPost("/dashboard/keys/"+targetKey.ID+"/update", url.Values{
		"name":       {"target-key"},
		"clear_rate": {"true"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("update clear_rate failed: %v", rec.Header().Get("Location"))
	}

	rec = sendPost("/dashboard/keys/"+targetKey.ID+"/update", url.Values{
		"name":          {"target-key"},
		"rate_requests": {"abc"},
		"rate_interval": {"1m"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error for non-integer rate requests on update")
	}

	rec = sendPost("/dashboard/keys/"+targetKey.ID+"/update", url.Values{
		"name":          {"target-key"},
		"rate_requests": {"10"},
		"rate_interval": {"bad-duration"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error for invalid rate interval on update")
	}

	// 7. Revoke error paths
	// Revoke unknown key ID
	rec = sendPost("/dashboard/keys/k_unknown_id/revoke", nil)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error when revoking unknown key ID")
	}

	// Revoke valid key once
	rec = sendPost("/dashboard/keys/"+targetKey.ID+"/revoke", nil)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("revoke key failed: %v", rec.Header().Get("Location"))
	}

	// Revoke already disabled key
	rec = sendPost("/dashboard/keys/"+targetKey.ID+"/revoke", nil)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error when revoking already revoked key")
	}

	// Test expiry_days in custom expiry
	rec = sendPost("/dashboard/keys/create", url.Values{
		"name":          {"bot-days"},
		"expiry_choice": {"custom"},
		"expiry_days":   {"90"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("create with expiry_days 90 failed: %v", rec.Header().Get("Location"))
	}

	// Test invalid expiry_days
	rec = sendPost("/dashboard/keys/create", url.Values{
		"name":          {"bot-bad-days"},
		"expiry_choice": {"custom"},
		"expiry_days":   {"-10"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error for negative expiry_days")
	}

	// Test enable_rate with value and units (h and d)
	rec = sendPost("/dashboard/keys/create", url.Values{
		"name":               {"bot-hours-rate"},
		"enable_rate":        {"true"},
		"rate_requests":      {"100"},
		"rate_interval_val":  {"2"},
		"rate_interval_unit": {"h"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("create with interval unit 'h' failed: %v", rec.Header().Get("Location"))
	}

	rec = sendPost("/dashboard/keys/create", url.Values{
		"name":               {"bot-days-rate"},
		"enable_rate":        {"true"},
		"rate_requests":      {"50"},
		"rate_interval_val":  {"1"},
		"rate_interval_unit": {"d"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("create with interval unit 'd' failed: %v", rec.Header().Get("Location"))
	}

	// Test enable_rate true with missing requests or invalid unit
	rec = sendPost("/dashboard/keys/create", url.Values{
		"name":        {"bot-missing-rate-reqs"},
		"enable_rate": {"true"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error for enable_rate true without requests")
	}

	rec = sendPost("/dashboard/keys/create", url.Values{
		"name":               {"bot-bad-unit"},
		"enable_rate":        {"true"},
		"rate_requests":      {"10"},
		"rate_interval_val":  {"1"},
		"rate_interval_unit": {"w"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("expected error for invalid interval unit 'w'")
	}

	// Update with enable_rate=false should clear rate limit
	rec = sendPost("/dashboard/keys/"+targetKey.ID+"/update", url.Values{
		"name":        {"target-key"},
		"enable_rate": {"false"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Errorf("update enable_rate false failed: %v", rec.Header().Get("Location"))
	}

	// Test fallback path value parsing and empty keysPath fallback
	h := NewHandler(deps)
	recDirect := httptest.NewRecorder()
	reqDirect := httptest.NewRequest(http.MethodPost, "/dashboard/keys/k_unknown_direct/revoke", nil)
	reqDirect.Host = "127.0.0.1:8787"
	h.handleKeyRevoke(recDirect, reqDirect)
	if recDirect.Code != http.StatusSeeOther {
		t.Errorf("expected redirect on direct handleKeyRevoke")
	}

	emptyDeps := &Deps{}
	hEmpty := NewHandler(emptyDeps)
	p := hEmpty.getKeysPath()
	if !strings.HasSuffix(p, "keys.json") {
		t.Errorf("expected default getKeysPath to end with keys.json, got %s", p)
	}
}
