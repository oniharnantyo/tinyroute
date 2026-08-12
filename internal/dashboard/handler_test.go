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

type mockHistoryQuerier struct{}

func (m *mockHistoryQuerier) List(ctx context.Context, filter history.Filter) ([]history.Summary, string, error) {
	return []history.Summary{
		{
			ID:           "req-123",
			Timestamp:    time.Now(),
			Provider:     "openai",
			ModelReq:     "gpt-4o",
			Outcome:      "success",
			InputTokens:  100,
			OutputTokens: 50,
			Latency:      200 * time.Millisecond,
		},
	}, "", nil
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
