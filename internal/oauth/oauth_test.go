package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/preset"
)

func TestGenerateHelpers(t *testing.T) {
	v, c, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("unexpected PKCE error: %v", err)
	}
	if v == "" || c == "" {
		t.Errorf("expected non-empty verifier and challenge, got v=%q, c=%q", v, c)
	}

	state := GenerateState()
	if len(state) == 0 {
		t.Errorf("expected non-empty state")
	}

	devID := GenerateDeviceID()
	if len(devID) == 0 {
		t.Errorf("expected non-empty device ID")
	}
}

func TestPKCEFlow(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			if r.FormValue("grant_type") != "authorization_code" {
				http.Error(w, "invalid grant_type", http.StatusBadRequest)
				return
			}
			if r.FormValue("code") != "valid_code" {
				http.Error(w, "invalid code", http.StatusBadRequest)
				return
			}
			res := map[string]interface{}{
				"access_token":  "acc_123",
				"refresh_token": "ref_123",
				"expires_in":    3600,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(res)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	p := &preset.Preset{
		Name:              "test-provider",
		ClientID:          "test-client-id",
		AuthorizeEndpoint: ts.URL + "/auth",
		TokenEndpoint:     ts.URL + "/token",
	}

	session, err := StartPKCE(p, "http://127.0.0.1:8787/callback")
	if err != nil {
		t.Fatalf("StartPKCE failed: %v", err)
	}
	if session.AuthorizeURL == "" || session.State == "" || session.Verifier == "" {
		t.Errorf("expected PKCESession fields to be populated, got %+v", session)
	}

	rec, err := ExchangePKCE(context.Background(), p, ts.Client(), "valid_code", session.Verifier, "http://127.0.0.1:8787/callback")
	if err != nil {
		t.Fatalf("ExchangePKCE failed: %v", err)
	}
	if rec.AccessToken != "acc_123" || rec.RefreshToken != "ref_123" {
		t.Errorf("unexpected OAuthRecord: %+v", rec)
	}
}

func TestDeviceFlow(t *testing.T) {
	pollCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/device" {
			res := map[string]interface{}{
				"device_code":      "dev_code_123",
				"user_code":        "ABCD-1234",
				"verification_uri": "https://example.com/device",
				"expires_in":       600,
				"interval":         5,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(res)
			return
		}
		if r.URL.Path == "/token" {
			pollCount++
			w.Header().Set("Content-Type", "application/json")
			if pollCount == 1 {
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token":  "dev_acc_456",
				"refresh_token": "dev_ref_456",
				"expires_in":    3600,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	p := &preset.Preset{
		Name:           "device-provider",
		ClientID:       "dev-client-id",
		DeviceEndpoint: ts.URL + "/device",
		TokenEndpoint:  ts.URL + "/token",
	}

	session, err := StartDeviceFlow(context.Background(), p, ts.Client())
	if err != nil {
		t.Fatalf("StartDeviceFlow failed: %v", err)
	}
	if session.DeviceCode != "dev_code_123" || session.UserCode != "ABCD-1234" {
		t.Errorf("unexpected DeviceSession: %+v", session)
	}

	// Poll 1: pending
	rec, pending, err := PollDeviceFlow(context.Background(), p, ts.Client(), session.DeviceCode, session.DeviceID)
	if err != nil || !pending || rec != nil {
		t.Errorf("expected pending=true on poll 1, got rec=%v, pending=%v, err=%v", rec, pending, err)
	}

	// Poll 2: success
	rec, pending, err = PollDeviceFlow(context.Background(), p, ts.Client(), session.DeviceCode, session.DeviceID)
	if err != nil || pending || rec == nil {
		t.Fatalf("expected success on poll 2, got rec=%v, pending=%v, err=%v", rec, pending, err)
	}
	if rec.AccessToken != "dev_acc_456" || rec.RefreshToken != "dev_ref_456" {
		t.Errorf("unexpected record on poll 2: %+v", rec)
	}
}

func TestClinePKCEFlow(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/authorize" {
			if r.URL.Query().Get("callback_url") == "" {
				http.Error(w, "missing callback_url", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/auth/token" {
			if r.Header.Get("Content-Type") != "application/json" {
				http.Error(w, "expected application/json", http.StatusBadRequest)
				return
			}
			var reqBody map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			if reqBody["grant_type"] != "authorization_code" || reqBody["code"] != "cline_code" || reqBody["client_type"] != "extension" {
				http.Error(w, "invalid params", http.StatusBadRequest)
				return
			}
			res := map[string]interface{}{
				"success": true,
				"data": map[string]interface{}{
					"accessToken":  "cline_access_token",
					"refreshToken": "cline_refresh_token",
					"tokenType":    "Bearer",
					"expiresAt":    "2026-12-31T23:59:59Z",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(res)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	p := &preset.Preset{
		Name:              "cline",
		AuthorizeEndpoint: ts.URL + "/auth/authorize",
		TokenEndpoint:     ts.URL + "/auth/token",
		ExtraParams: map[string]string{
			"client_type": "extension",
		},
	}

	session, err := StartPKCE(p, "http://127.0.0.1:8787/dashboard/oauth/callback")
	if err != nil {
		t.Fatalf("StartPKCE failed: %v", err)
	}
	if !strings.Contains(session.AuthorizeURL, "callback_url=") {
		t.Fatalf("expected callback_url in authorize URL: %s", session.AuthorizeURL)
	}

	rec, err := ExchangePKCE(context.Background(), p, ts.Client(), "cline_code", session.Verifier, "http://127.0.0.1:8787/dashboard/oauth/callback")
	if err != nil {
		t.Fatalf("ExchangePKCE failed: %v", err)
	}
	if rec.AccessToken != "workos:cline_access_token" || rec.RefreshToken != "cline_refresh_token" {
		t.Errorf("unexpected record: %+v", rec)
	}
	if rec.ExpiresAt.IsZero() {
		t.Errorf("expected parsed ExpiresAt, got zero")
	}
}
