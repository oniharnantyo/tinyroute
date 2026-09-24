package dashboard_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/dashboard"
)

func newTestReq(method, path string, body []byte) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Host = "127.0.0.1:8787"
	return req
}

func TestAPIAuthAndOverview(t *testing.T) {
	mux, sessID, _ := setupTestDashboard(t)

	// 1. Unauthenticated status check
	req := newTestReq("GET", "/dashboard/api/auth/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var statusRes map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &statusRes); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if statusRes["authenticated"] != false {
		t.Errorf("expected authenticated=false, got %v", statusRes["authenticated"])
	}

	// 2. Unauthenticated request to protected API endpoint -> 401 Unauthorized
	req = newTestReq("GET", "/dashboard/api/overview", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}

	// 3. Login with correct password (default is 123456)
	loginPayload, _ := json.Marshal(map[string]string{"password": "123456"})
	req = newTestReq("POST", "/dashboard/api/auth/login", loginPayload)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected login status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	cookie := rec.Result().Cookies()
	if len(cookie) == 0 || cookie[0].Name != dashboard.SessionCookieName {
		t.Fatalf("expected session cookie in login response")
	}

	// 4. Authenticated request to /dashboard/api/overview
	req = newTestReq("GET", "/dashboard/api/overview?window=24h", nil)
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for overview API, got %d: %s", rec.Code, rec.Body.String())
	}

	var overviewRes map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &overviewRes); err != nil {
		t.Fatalf("failed to decode overview JSON: %v", err)
	}
	if overviewRes["active_window"] != "24h" {
		t.Errorf("expected active_window 24h, got %v", overviewRes["active_window"])
	}
}

func TestAPIProvidersAndModels(t *testing.T) {
	mux, sessID, _ := setupTestDashboard(t)

	// 1. Get Providers
	req := newTestReq("GET", "/dashboard/api/providers", nil)
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var provRes map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &provRes); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	presets, ok := provRes["presets"].([]any)
	if !ok || len(presets) == 0 {
		t.Errorf("expected presets list to be populated")
	}

	// 2. Add Provider
	addPayload, _ := json.Marshal(map[string]string{
		"name":   "groq",
		"preset": "groq",
	})
	req = newTestReq("POST", "/dashboard/api/providers/add", addPayload)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on add provider, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Add Model
	modelPayload, _ := json.Marshal(map[string]string{
		"provider": "groq",
		"model":    "llama-3.3-70b-versatile",
	})
	req = newTestReq("POST", "/dashboard/api/models/add", modelPayload)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on add model, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Remove Model
	req = newTestReq("POST", "/dashboard/api/models/remove", modelPayload)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on remove model, got %d", rec.Code)
	}

	// 5. Delete Provider
	delPayload, _ := json.Marshal(map[string]string{"name": "groq"})
	req = newTestReq("POST", "/dashboard/api/providers/delete", delPayload)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on delete provider, got %d", rec.Code)
	}
}

func TestAPIKeysAndCombos(t *testing.T) {
	mux, sessID, _ := setupTestDashboard(t)

	// 1. Get Keys
	req := newTestReq("GET", "/dashboard/api/keys", nil)
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on get keys, got %d", rec.Code)
	}

	// 2. Create Key
	rpm := 120
	keyPayload, _ := json.Marshal(map[string]any{
		"name":           "test-agent-key",
		"rate_limit_rpm": rpm,
	})
	req = newTestReq("POST", "/dashboard/api/keys/create", keyPayload)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on create key, got %d: %s", rec.Code, rec.Body.String())
	}

	var keyCreatedRes map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &keyCreatedRes); err != nil {
		t.Fatalf("failed to unmarshal key create response: %v", err)
	}
	secret, ok := keyCreatedRes["key"].(string)
	if !ok || secret == "" {
		t.Fatalf("expected secret token returned")
	}

	// 3. Add Provider first so combo validation succeeds
	addProvPayload, _ := json.Marshal(map[string]string{
		"name":   "openai",
		"preset": "openai",
	})
	req = newTestReq("POST", "/dashboard/api/providers/add", addProvPayload)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on add provider: %s", rec.Body.String())
	}

	// 4. Create Combo
	comboPayload, _ := json.Marshal(map[string]any{
		"name": "fast-combo",
		"routes": []map[string]string{
			{"provider": "openai", "model": "gpt-4o-mini"},
		},
	})
	req = newTestReq("POST", "/dashboard/api/combos/wizard", comboPayload)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on create combo, got %d: %s", rec.Code, rec.Body.String())
	}

	// 5. Toggle Combo
	togglePayload, _ := json.Marshal(map[string]any{
		"name":    "fast-combo",
		"enabled": false,
	})
	req = newTestReq("POST", "/dashboard/api/combos/toggle", togglePayload)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on toggle combo, got %d: %s", rec.Code, rec.Body.String())
	}

	// 6. Delete Combo
	delComboPayload := map[string]string{"name": "fast-combo"}
	delComboBytes, _ := json.Marshal(delComboPayload)
	req = newTestReq("POST", "/dashboard/api/combos/delete", delComboBytes)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 on delete combo, got %d", rec.Code)
	}
}

func TestAPIHistoryAndClientsAndSettings(t *testing.T) {
	mux, sessID, _ := setupTestDashboard(t)

	// 1. History list
	req := newTestReq("GET", "/dashboard/api/history?limit=10", nil)
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for history API, got %d", rec.Code)
	}

	// 2. Clients list
	req = newTestReq("GET", "/dashboard/api/clients", nil)
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for clients API, got %d", rec.Code)
	}

	// 3. Settings info
	req = newTestReq("GET", "/dashboard/api/settings", nil)
	req.AddCookie(&http.Cookie{Name: dashboard.SessionCookieName, Value: sessID})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for settings API, got %d", rec.Code)
	}
}
