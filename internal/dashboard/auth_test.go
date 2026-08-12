package dashboard

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestPasswordStoreSeedAndVerify(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "dashboard.json")

	store, err := NewPasswordStore(path)
	if err != nil {
		t.Fatalf("failed to create password store: %v", err)
	}

	if !store.IsDefaultPassword() {
		t.Errorf("expected new store to have default password flag set")
	}

	if !store.VerifyPassword("123456") {
		t.Errorf("expected default password '123456' to verify")
	}

	if store.VerifyPassword("wrongpass") {
		t.Errorf("expected wrong password to fail verification")
	}

	// Change password
	if err := store.SetPassword("newsecret123"); err != nil {
		t.Fatalf("failed to set new password: %v", err)
	}

	if store.IsDefaultPassword() {
		t.Errorf("expected isDefault to be false after password change")
	}

	if !store.VerifyPassword("newsecret123") {
		t.Errorf("expected new password to verify")
	}

	if store.VerifyPassword("123456") {
		t.Errorf("expected old password to fail verification")
	}

	// Reload store from disk
	reloaded, err := NewPasswordStore(path)
	if err != nil {
		t.Fatalf("failed to reload password store: %v", err)
	}

	if !reloaded.VerifyPassword("newsecret123") {
		t.Errorf("expected reloaded store to verify new password")
	}
}

func TestSessionStore(t *testing.T) {
	ss := NewSessionStore()

	if ss.ValidateSession("invalid-token") {
		t.Errorf("expected invalid token to fail validation")
	}

	token := ss.CreateSession(1 * time.Hour)
	if token == "" {
		t.Fatalf("expected non-empty token")
	}

	if !ss.ValidateSession(token) {
		t.Errorf("expected valid session token to pass validation")
	}

	ss.RevokeSession(token)
	if ss.ValidateSession(token) {
		t.Errorf("expected revoked session token to fail validation")
	}
}

func TestIsLoopbackHost(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{"localhost", true},
		{"localhost:8787", true},
		{"127.0.0.1", true},
		{"127.0.0.1:8787", true},
		{"::1", true},
		{"[::1]:8787", true},
		{"example.com", false},
		{"192.168.1.1", false},
		{"10.0.0.1:8787", false},
	}

	for _, tt := range tests {
		got := IsLoopbackHost(tt.host)
		if got != tt.expected {
			t.Errorf("IsLoopbackHost(%q) = %v, want %v", tt.host, got, tt.expected)
		}
	}
}

func TestHostGuardMiddleware(t *testing.T) {
	handler := HostGuardMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// GET request allowed regardless of Host
	req := httptest.NewRequest(http.MethodGet, "/dashboard/overview", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected GET request to be allowed, got status %d", rec.Code)
	}

	// POST from loopback Host allowed
	req = httptest.NewRequest(http.MethodPost, "/dashboard/models/add", nil)
	req.Host = "127.0.0.1:8787"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected loopback POST to be allowed, got status %d", rec.Code)
	}

	// POST from non-loopback Host rejected
	req = httptest.NewRequest(http.MethodPost, "/dashboard/models/add", nil)
	req.Host = "attacker.com"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected non-loopback POST to be forbidden, got status %d", rec.Code)
	}

	// POST with non-loopback Origin rejected
	req = httptest.NewRequest(http.MethodPost, "/dashboard/models/add", nil)
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Origin", "http://evil-site.com")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected non-loopback Origin POST to be forbidden, got status %d", rec.Code)
	}
}

func TestLoginLimiter(t *testing.T) {
	limiter := NewLoginLimiter()
	ip := "127.0.0.1"

	for i := 0; i < 5; i++ {
		allowed, _ := limiter.Allow(ip)
		if !allowed {
			t.Errorf("expected attempt %d to be allowed", i+1)
		}
	}

	allowed, retryAfter := limiter.Allow(ip)
	if allowed {
		t.Errorf("expected 6th login attempt to be throttled")
	}
	if retryAfter <= 0 {
		t.Errorf("expected positive retry after duration")
	}
}
