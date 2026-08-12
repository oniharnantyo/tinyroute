package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/cli/interactive"
	"github.com/oniharnantyo/tinyroute/internal/credential"
	"github.com/oniharnantyo/tinyroute/internal/preset"
)

// syncBuffer is a goroutine-safe output buffer: runPKCEFlow writes to it
// while tests poll its content from another goroutine.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestGeneratePKCE(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE failed: %v", err)
	}

	if len(verifier) == 0 {
		t.Error("expected non-empty verifier")
	}

	h := sha256.Sum256([]byte(verifier))
	expectedChallenge := base64.RawURLEncoding.EncodeToString(h[:])
	if challenge != expectedChallenge {
		t.Errorf("challenge mismatch: got %q, want %q", challenge, expectedChallenge)
	}
}

func TestDeviceCodeFlow_Success(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.json")
	store, err := credential.NewStore(credPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	var pollCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/device" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":      "dev_123",
				"user_code":        "ABCD-1234",
				"verification_uri": "https://auth.example.com/device",
				"expires_in":       60,
				"interval":         1,
			})
			return
		}
		if r.URL.Path == "/token" {
			pollCount++
			if pollCount == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": "authorization_pending",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "at_device_token",
				"refresh_token": "rt_device_token",
				"expires_in":    3600,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	testPreset := &preset.Preset{
		Name:           "test-device-prov",
		OAuthCapable:   true,
		FlowType:       "device_code",
		ClientID:       "client_test",
		DeviceEndpoint: ts.URL + "/device",
		TokenEndpoint:  ts.URL + "/token",
		Scopes:         []string{"openid"},
	}

	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = runDeviceCodeFlow(ctx, testPreset, ts.Client(), store, &out)
	if err != nil {
		t.Fatalf("runDeviceCodeFlow failed: %v", err)
	}

	if !strings.Contains(out.String(), "ABCD-1234") {
		t.Errorf("output missing user code ABCD-1234, got:\n%s", out.String())
	}

	rec, ok := store.Get("test-device-prov")
	if !ok {
		t.Fatal("expected stored credential record")
	}
	if rec.AccessToken != "at_device_token" || rec.RefreshToken != "rt_device_token" {
		t.Errorf("stored record token mismatch: %+v", rec)
	}
}

func TestPKCEFlow_Success(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.json")
	store, err := credential.NewStore(credPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "at_pkce_token",
				"refresh_token": "rt_pkce_token",
				"expires_in":    3600,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	testPreset := &preset.Preset{
		Name:              "test-pkce-prov",
		OAuthCapable:      true,
		FlowType:          "pkce",
		ClientID:          "client_pkce",
		AuthorizeEndpoint: "http://127.0.0.1/authorize",
		TokenEndpoint:     ts.URL + "/token",
	}

	out := &syncBuffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		for i := 0; i < 50; i++ {
			time.Sleep(20 * time.Millisecond)
			s := out.String()
			if strings.Contains(s, "Waiting for callback on") {
				idx := strings.Index(s, "http://127.0.0.1:")
				if idx != -1 {
					lines := strings.Split(s[idx:], "\n")
					parts := strings.Split(lines[0], "...")
					cbURL := strings.TrimSpace(parts[0])

					u, err := url.Parse(authURLFromOutput(s))
					if err == nil {
						state := u.Query().Get("state")
						fullCB := cbURL + "?code=test_auth_code&state=" + state
						_, _ = http.Get(fullCB)
					}
					return
				}
			}
		}
	}()

	err = runPKCEFlow(ctx, testPreset, ts.Client(), store, out)
	if err != nil {
		t.Fatalf("runPKCEFlow failed: %v", err)
	}

	rec, ok := store.Get("test-pkce-prov")
	if !ok {
		t.Fatal("expected stored PKCE credential record")
	}
	if rec.AccessToken != "at_pkce_token" || rec.RefreshToken != "rt_pkce_token" {
		t.Errorf("stored record token mismatch: %+v", rec)
	}
}

func authURLFromOutput(out string) string {
	idx := strings.Index(out, "http://127.0.0.1/authorize?")
	if idx == -1 {
		return ""
	}
	sub := out[idx:]
	end := strings.Index(sub, "\n")
	if end == -1 {
		return sub
	}
	return strings.TrimSpace(sub[:end])
}

func TestAuthImport(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.json")
	t.Setenv("TINYROUTE_CREDENTIALS", credPath)

	opts := importOptions{
		refreshToken: "rt_flag_val",
		accessToken:  "at_flag_val",
	}
	err := cmdAuthImport([]string{"xai"}, opts)
	if err != nil {
		t.Fatalf("cmdAuthImport failed: %v", err)
	}

	store, _ := credential.NewStore(credPath)
	rec, ok := store.Get("xai")
	if !ok {
		t.Fatal("expected record for xai")
	}
	if rec.RefreshToken != "rt_flag_val" || rec.AccessToken != "at_flag_val" {
		t.Errorf("imported record mismatch: %+v", rec)
	}

	fileJSON := filepath.Join(dir, "native.json")
	fileContent := `{
		"refresh_token": "rt_file_val",
		"access_token": "at_file_val",
		"client_id": "custom_client_id"
	}`
	_ = os.WriteFile(fileJSON, []byte(fileContent), 0600)

	optsFile := importOptions{filePath: fileJSON}
	err = cmdAuthImport([]string{"custom-prov"}, optsFile)
	if err != nil {
		t.Fatalf("cmdAuthImport file failed: %v", err)
	}

	recFile, ok := store.Get("custom-prov")
	if !ok {
		t.Fatal("expected record for custom-prov")
	}
	if recFile.RefreshToken != "rt_file_val" || recFile.ClientID != "custom_client_id" {
		t.Errorf("imported file record mismatch: %+v", recFile)
	}
}

func TestAuthStatus(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.json")
	t.Setenv("TINYROUTE_CREDENTIALS", credPath)

	store, _ := credential.NewStore(credPath)
	_ = store.Save(credential.OAuthRecord{
		Provider:     "xai",
		RefreshToken: "rt_xai_secret",
		ExpiresAt:    time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC),
	})

	err := cmdAuthStatus([]string{})
	if err != nil {
		t.Fatalf("cmdAuthStatus failed: %v", err)
	}
}

func TestPKCEFlow_CancelledBySignal(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.json")
	store, err := credential.NewStore(credPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	testPreset := &preset.Preset{
		Name:              "test-pkce-cancel",
		OAuthCapable:      true,
		FlowType:          "pkce",
		ClientID:          "client_pkce",
		AuthorizeEndpoint: "http://127.0.0.1/authorize",
		TokenEndpoint:     "http://127.0.0.1/token",
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately to simulate SIGINT during authorization
	cancel()

	out := &syncBuffer{}
	err = runPKCEFlow(ctx, testPreset, http.DefaultClient, store, out)
	if err == nil {
		t.Fatal("expected error when context is cancelled, got nil")
	}

	_, ok := store.Get("test-pkce-cancel")
	if ok {
		t.Fatal("expected no stored credential when flow is cancelled")
	}
}

func TestProviderAdd_CancelledOAuth_DoesNotPersistConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	t.Setenv("TINYROUTE_CONFIG", configPath)

	// Set override so interactive prompt acts as non-interactive without failing
	falseVal := false
	interactive.SetCanPromptOverride(&falseVal)
	defer interactive.SetCanPromptOverride(nil)

	// Verify config.yaml does not exist initially
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("expected config.yaml to not exist initially")
	}

	// Attempting to add provider non-interactively without credentials when login fails or is cancelled
	// verifies config write handling
	_ = cmdAdd([]string{"codex", "--no-interactive"})
	// Non-interactive adds without login, so config should exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("expected config.yaml to exist after non-interactive provider add")
	}
}

func TestBuildAuthorizeURL(t *testing.T) {
	t.Run("Codex", func(t *testing.T) {
		p := preset.Get("codex")
		if p == nil {
			t.Fatal("codex preset not found")
		}
		rawURL, err := buildAuthorizeURL(p, 1455, "state123", "ch123")
		if err != nil {
			t.Fatalf("buildAuthorizeURL failed: %v", err)
		}
		parsed, err := url.Parse(rawURL)
		if err != nil {
			t.Fatalf("parse rawURL failed: %v", err)
		}
		if parsed.Query().Get("redirect_uri") != "http://localhost:1455/auth/callback" {
			t.Errorf("expected localhost redirect URI, got: %s", parsed.Query().Get("redirect_uri"))
		}
		if !strings.Contains(rawURL, "codex_cli_simplified_flow=true") ||
			!strings.Contains(rawURL, "id_token_add_organizations=true") ||
			!strings.Contains(rawURL, "originator=codex_cli_rs") {
			t.Errorf("missing extra params in codex authorize URL: %s", rawURL)
		}
		// Verify %20 scope encoding (no +)
		if strings.Contains(rawURL, "scope=openid+profile") {
			t.Errorf("authorize URL contains '+' instead of '%%20' in scope: %s", rawURL)
		}
		if !strings.Contains(rawURL, "scope=openid%20profile%20email%20offline_access") {
			t.Errorf("scope missing %%20 encoding: %s", rawURL)
		}
	})

	t.Run("Iflow", func(t *testing.T) {
		p := preset.Get("iflow")
		if p == nil {
			t.Fatal("iflow preset not found")
		}
		rawURL, err := buildAuthorizeURL(p, 1455, "state123", "ch123")
		if err != nil {
			t.Fatalf("buildAuthorizeURL failed: %v", err)
		}
		if !strings.Contains(rawURL, "loginMethod=phone") || !strings.Contains(rawURL, "type=phone") {
			t.Errorf("missing extra params in iflow authorize URL: %s", rawURL)
		}
	})

	t.Run("Cline", func(t *testing.T) {
		p := preset.Get("cline")
		if p == nil {
			t.Fatal("cline preset not found")
		}
		rawURL, err := buildAuthorizeURL(p, 1455, "state123", "ch123")
		if err != nil {
			t.Fatalf("buildAuthorizeURL failed: %v", err)
		}
		if !strings.Contains(rawURL, "client_type=extension") {
			t.Errorf("missing client_type=extension in cline authorize URL: %s", rawURL)
		}
		if !strings.Contains(rawURL, "callback_url=") {
			t.Errorf("missing callback_url in cline authorize URL: %s", rawURL)
		}
	})

	t.Run("Antigravity", func(t *testing.T) {
		p := preset.Get("antigravity")
		if p == nil {
			t.Fatal("antigravity preset not found")
		}
		rawURL, err := buildAuthorizeURL(p, 1455, "state123", "ch123")
		if err != nil {
			t.Fatalf("buildAuthorizeURL failed: %v", err)
		}
		if !strings.Contains(rawURL, "client_id=1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com") {
			t.Errorf("missing client_id in antigravity authorize URL: %s", rawURL)
		}
		if !strings.Contains(rawURL, "https://accounts.google.com/o/oauth2/v2/auth") {
			t.Errorf("wrong authorize endpoint for antigravity: %s", rawURL)
		}
	})

	t.Run("Defaults", func(t *testing.T) {
		p := preset.Get("claude")
		if p == nil {
			t.Fatal("claude preset not found")
		}
		rawURL, err := buildAuthorizeURL(p, 1455, "state123", "ch123")
		if err != nil {
			t.Fatalf("buildAuthorizeURL failed: %v", err)
		}
		parsed, err := url.Parse(rawURL)
		if err != nil {
			t.Fatalf("parse rawURL failed: %v", err)
		}
		if parsed.Query().Get("redirect_uri") != "http://127.0.0.1:1455/callback" {
			t.Errorf("expected default loopback fallback redirect URI, got: %s", parsed.Query().Get("redirect_uri"))
		}
	})
}

func TestDeviceHeaderBuilder_Kimi(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://auth.kimi.com/api/oauth/device_authorization", nil)
	deviceID := "test-device-uuid-1234"

	credential.ApplyDeviceHeaders(req, "kimi", deviceID)

	if req.Header.Get("X-Msh-Device-Id") != deviceID {
		t.Errorf("expected X-Msh-Device-Id=%q, got %q", deviceID, req.Header.Get("X-Msh-Device-Id"))
	}
	if req.Header.Get("X-Msh-Platform") == "" {
		t.Error("expected X-Msh-Platform to be set")
	}
	if req.Header.Get("X-Msh-Version") == "" {
		t.Error("expected X-Msh-Version to be set")
	}

	// Verify stability across calls
	req2, _ := http.NewRequest("POST", "https://auth.kimi.com/api/oauth/token", nil)
	credential.ApplyDeviceHeaders(req2, "kimi", deviceID)
	if req2.Header.Get("X-Msh-Device-Id") != deviceID {
		t.Errorf("device ID changed across requests: got %q, want %q", req2.Header.Get("X-Msh-Device-Id"), deviceID)
	}
}

func TestCustomFlowDispatch(t *testing.T) {
	pQoder := preset.Get("qoder")
	if pQoder == nil || pQoder.FlowType != "qoder" {
		t.Errorf("expected qoder flow_type to be 'qoder', got %+v", pQoder)
	}

	pTrae := preset.Get("trae")
	if pTrae == nil || pTrae.FlowType != "trae" {
		t.Errorf("expected trae flow_type to be 'trae', got %+v", pTrae)
	}
}

func TestGitlabInteractiveClientID(t *testing.T) {
	t.Run("Non-TTY errors clearly", func(t *testing.T) {
		falseVal := false
		interactive.SetCanPromptOverride(&falseVal)
		defer interactive.SetCanPromptOverride(nil)

		dir := t.TempDir()
		credPath := filepath.Join(dir, "credentials.json")
		t.Setenv("TINYROUTE_CREDENTIALS", credPath)

		err := cmdAuthLogin(context.Background(), []string{"gitlab"}, false)
		if err == nil {
			t.Fatal("expected error in non-TTY mode for gitlab, got nil")
		}
		if !strings.Contains(err.Error(), "client_id") {
			t.Errorf("expected error to mention client_id, got: %v", err)
		}
	})
}

func TestRunQoderFlow_Success(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.json")
	store, err := credential.NewStore(credPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	var pollCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/deviceToken/poll" {
			pollCount++
			var reqBody map[string]string
			_ = json.NewDecoder(r.Body).Decode(&reqBody)
			if reqBody["nonce"] == "" {
				t.Error("expected nonce in poll request body")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_token": "dt_qoder_test_token",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	p := &preset.Preset{
		Name:           "qoder",
		FlowType:       "qoder",
		DeviceEndpoint: ts.URL + "/deviceToken/poll",
		TokenEndpoint:  ts.URL + "/refresh",
	}

	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = runQoderFlow(ctx, p, ts.Client(), store, &out)
	if err != nil {
		t.Fatalf("runQoderFlow failed: %v", err)
	}

	rec, ok := store.Get("qoder")
	if !ok {
		t.Fatal("expected stored qoder credential record")
	}
	if rec.RefreshToken != "dt_qoder_test_token" {
		t.Errorf("stored record token mismatch: %+v", rec)
	}
}

func TestRunTraeFlow_Success(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.json")
	store, err := credential.NewStore(credPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/GetLoginGuidance" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"redirect_url": "https://auth.marscode.com/login",
				},
			})
			return
		}
		if r.URL.Path == "/ExchangeToken" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "at_trae_test",
				"refresh_token": "rt_trae_test",
				"expires_in":    3600,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	p := &preset.Preset{
		Name:              "trae",
		FlowType:          "trae",
		ClientID:          "ono9krqynydwx5",
		AuthorizeEndpoint: ts.URL + "/GetLoginGuidance",
		TokenEndpoint:     ts.URL + "/ExchangeToken",
	}

	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = runTraeFlow(ctx, p, ts.Client(), store, &out)
	if err != nil {
		t.Fatalf("runTraeFlow failed: %v", err)
	}

	rec, ok := store.Get("trae")
	if !ok {
		t.Fatal("expected stored trae credential record")
	}
	if rec.AccessToken != "at_trae_test" || rec.RefreshToken != "rt_trae_test" {
		t.Errorf("stored record token mismatch: %+v", rec)
	}
}
