package credential

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStaticKey_ReturnsValueImmediately(t *testing.T) {
	keyVal := "sk-test-static-key-12345"
	cred := NewStaticKey(keyVal)

	res, err := cred.Token(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if res.Value != keyVal {
		t.Errorf("expected token value %q, got %q", keyVal, res.Value)
	}
	if res.Kind != KindStatic {
		t.Errorf("expected token kind %q, got %q", KindStatic, res.Kind)
	}
}

func TestRefreshDedup_CollapsesConcurrentCalls(t *testing.T) {
	var callCount int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		time.Sleep(50 * time.Millisecond) // simulate network delay
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at_dedup_success",
			"expires_in":   3600,
		})
	}))
	defer ts.Close()

	rm := NewRefreshManager(ts.Client())
	ctx := context.Background()
	req := RefreshRequest{
		Provider:      "xai",
		RefreshToken:  "rt_concurrent_test",
		ClientID:      "client_123",
		TokenEndpoint: ts.URL,
		Profile:       RefreshProfile{BodyFormat: FormatJSON},
	}

	const goroutines = 50
	var wg sync.WaitGroup
	results := make([]TokenResult, goroutines)
	errors := make([]error, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		idx := i
		go func() {
			defer wg.Done()
			res, _, _, err := rm.RefreshToken(ctx, req)
			results[idx] = res
			errors[idx] = err
		}()
	}
	wg.Wait()

	if atomic.LoadInt64(&callCount) != 1 {
		t.Errorf("expected exactly 1 server call due to singleflight dedup, got %d", atomic.LoadInt64(&callCount))
	}

	for i := 0; i < goroutines; i++ {
		if errors[i] != nil {
			t.Errorf("goroutine %d returned error: %v", i, errors[i])
		}
		if results[i].Value != "at_dedup_success" {
			t.Errorf("goroutine %d got token %q, expected %q", i, results[i].Value, "at_dedup_success")
		}
		if results[i].Kind != KindOAuthBearer {
			t.Errorf("goroutine %d got kind %q, expected %q", i, results[i].Kind, KindOAuthBearer)
		}
	}
}

func TestResultCacheHit_10s(t *testing.T) {
	var callCount int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at_cached_token",
			"expires_in":   3600,
		})
	}))
	defer ts.Close()

	rm := NewRefreshManager(ts.Client())
	ctx := context.Background()
	req := RefreshRequest{
		Provider:      "xai",
		RefreshToken:  "rt_cache_test",
		ClientID:      "client_123",
		TokenEndpoint: ts.URL,
	}

	res1, _, _, err1 := rm.RefreshToken(ctx, req)
	if err1 != nil {
		t.Fatalf("first refresh failed: %v", err1)
	}

	res2, _, _, err2 := rm.RefreshToken(ctx, req)
	if err2 != nil {
		t.Fatalf("second refresh failed: %v", err2)
	}

	if atomic.LoadInt64(&callCount) != 1 {
		t.Errorf("expected 1 call due to 10s cache hit, got %d", atomic.LoadInt64(&callCount))
	}

	if res1.Value != res2.Value || res1.Value != "at_cached_token" {
		t.Errorf("cache hit mismatch: res1=%q res2=%q", res1.Value, res2.Value)
	}
}

func TestRefreshFailure_DoesNotPoisonCache(t *testing.T) {
	var shouldFail int32 = 1
	var callCount int64

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		if atomic.LoadInt32(&shouldFail) == 1 {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at_recovered_token",
			"expires_in":   3600,
		})
	}))
	defer ts.Close()

	rm := NewRefreshManager(ts.Client())
	ctx := context.Background()
	req := RefreshRequest{
		Provider:      "xai",
		RefreshToken:  "rt_poison_test",
		ClientID:      "client_123",
		TokenEndpoint: ts.URL,
	}

	_, _, _, err1 := rm.RefreshToken(ctx, req)
	if err1 == nil {
		t.Fatal("expected first refresh call to fail")
	}

	atomic.StoreInt32(&shouldFail, 0)

	res2, _, _, err2 := rm.RefreshToken(ctx, req)
	if err2 != nil {
		t.Fatalf("second refresh after failure recovery failed: %v", err2)
	}

	if res2.Value != "at_recovered_token" {
		t.Errorf("expected token %q, got %q", "at_recovered_token", res2.Value)
	}

	if atomic.LoadInt64(&callCount) != 2 {
		t.Errorf("expected 2 server calls (failed call not cached), got %d", atomic.LoadInt64(&callCount))
	}
}

func TestCredentialsFile_Mode0600(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "credentials.json")

	store, err := NewStore(filePath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	rec := OAuthRecord{
		Provider:      "xai",
		RefreshToken:  "rt_secret_123",
		AccessToken:   "at_secret_abc",
		ClientID:      "client_xai",
		TokenEndpoint: "https://auth.x.ai/oauth2/token",
	}

	if err := store.Save(rec); err != nil {
		t.Fatalf("failed to save credential: %v", err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat credentials.json: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected credentials.json mode 0600, got %o", perm)
	}

	fetched, ok := store.Get("xai")
	if !ok {
		t.Fatal("expected to find saved provider xai")
	}

	if fetched.RefreshToken != rec.RefreshToken || fetched.AccessToken != rec.AccessToken {
		t.Errorf("fetched record mismatch: got %+v, expected %+v", fetched, rec)
	}
}

func TestRefreshProfiles_JSON_Form_BasicAuth(t *testing.T) {
	ctx := context.Background()

	// 1. JSON profile
	reqJSON := RefreshRequest{
		Provider:      "claude",
		RefreshToken:  "rt_claude",
		ClientID:      "client_claude",
		TokenEndpoint: "https://auth.anthropic.com/token",
		Profile: RefreshProfile{
			BodyFormat:          FormatJSON,
			IncludeClientSecret: false,
		},
	}
	httpReqJSON, err := BuildRefreshRequest(ctx, reqJSON)
	if err != nil {
		t.Fatalf("failed to build JSON refresh request: %v", err)
	}
	if httpReqJSON.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", httpReqJSON.Header.Get("Content-Type"))
	}

	// 2. Basic Auth profile
	reqBasic := RefreshRequest{
		Provider:      "iflow",
		RefreshToken:  "rt_iflow",
		ClientID:      "id_iflow",
		ClientSecret:  "secret_iflow",
		TokenEndpoint: "https://auth.iflow.com/token",
		Profile: RefreshProfile{
			BodyFormat:   FormatForm,
			UseBasicAuth: true,
		},
	}
	httpReqBasic, err := BuildRefreshRequest(ctx, reqBasic)
	if err != nil {
		t.Fatalf("failed to build Basic auth refresh request: %v", err)
	}
	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("id_iflow:secret_iflow"))
	if httpReqBasic.Header.Get("Authorization") != expectedAuth {
		t.Errorf("expected Authorization %q, got %q", expectedAuth, httpReqBasic.Header.Get("Authorization"))
	}

	// 3. Cline profile (camelCase body & /auth/refresh mapping)
	reqCline := RefreshRequest{
		Provider:      "cline",
		RefreshToken:  "rt_cline",
		TokenEndpoint: "https://api.cline.bot/api/v1/auth/token",
		Profile: RefreshProfile{
			BodyFormat: FormatJSON,
		},
	}
	httpReqCline, err := BuildRefreshRequest(ctx, reqCline)
	if err != nil {
		t.Fatalf("failed to build Cline refresh request: %v", err)
	}
	if httpReqCline.URL.String() != "https://api.cline.bot/api/v1/auth/refresh" {
		t.Errorf("expected URL https://api.cline.bot/api/v1/auth/refresh, got %q", httpReqCline.URL.String())
	}
	var clineBody map[string]string
	if err := json.NewDecoder(httpReqCline.Body).Decode(&clineBody); err != nil {
		t.Fatalf("failed to decode Cline refresh body: %v", err)
	}
	if clineBody["grantType"] != "refresh_token" || clineBody["refreshToken"] != "rt_cline" {
		t.Errorf("unexpected Cline refresh body: %+v", clineBody)
	}
}

func TestOAuthRefreshable_Token_And_ProactiveRefresh(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "credentials.json")
	store, err := NewStore(filePath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at_refreshed_val",
			"expires_in":    3600,
			"refresh_token": "rt_new_rotated",
		})
	}))
	defer ts.Close()

	rm := NewRefreshManager(ts.Client())

	// 1. Valid cached token -> no refresh
	credValid := NewOAuthRefreshable(OAuthRefreshableConfig{
		Provider:       "xai",
		RefreshToken:   "rt_valid",
		AccessToken:    "at_existing_valid",
		ExpiresAt:      time.Now().Add(1 * time.Hour),
		LeadTime:       5 * time.Minute,
		RefreshManager: rm,
		Store:          store,
		HTTPClient:     ts.Client(),
	})
	tok, err := credValid.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.Value != "at_existing_valid" {
		t.Errorf("expected unexpired token %q, got %q", "at_existing_valid", tok.Value)
	}

	// 2. Expired token -> proactive refresh & store save
	credExpired := NewOAuthRefreshable(OAuthRefreshableConfig{
		Provider:       "xai",
		RefreshToken:   "rt_old",
		ClientID:       "client_xai",
		TokenEndpoint:  ts.URL,
		ExpiresAt:      time.Now().Add(-1 * time.Minute), // expired
		RefreshManager: rm,
		Store:          store,
		HTTPClient:     ts.Client(),
	})
	tokRefreshed, err := credExpired.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on expired token: %v", err)
	}
	if tokRefreshed.Value != "at_refreshed_val" {
		t.Errorf("expected refreshed token %q, got %q", "at_refreshed_val", tokRefreshed.Value)
	}

	saved, ok := store.Get("xai")
	if !ok {
		t.Fatal("expected updated credential record in store")
	}
	if saved.AccessToken != "at_refreshed_val" || saved.RefreshToken != "rt_new_rotated" {
		t.Errorf("store record not updated properly: %+v", saved)
	}
}

func TestStore_HotReload_And_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "credentials.json")

	store, err := NewStore(filePath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	rec := OAuthRecord{Provider: "qwen", RefreshToken: "rt_qwen"}
	if err := store.Save(rec); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	// Overwrite file with invalid JSON
	_ = os.WriteFile(filePath, []byte("{invalid json corrupt file"), 0o600)

	// Fetch should keep previous valid snapshot
	fetched, ok := store.Get("qwen")
	if !ok {
		t.Fatal("expected store to return last valid snapshot for qwen after file corruption")
	}
	if fetched.RefreshToken != "rt_qwen" {
		t.Errorf("expected refreshToken %q, got %q", "rt_qwen", fetched.RefreshToken)
	}
}

func TestStore_ListMasked(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "credentials.json")

	store, err := NewStore(filePath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	rec := OAuthRecord{
		Provider:     "claude",
		RefreshToken: "rt_sensitive_long_token_12345",
		AccessToken:  "at_sensitive_access_token_67890",
		ClientSecret: "cs_secret_val",
	}
	if err := store.Save(rec); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	rawList := store.List()
	if len(rawList) != 1 {
		t.Fatalf("expected 1 raw record, got %d", len(rawList))
	}
	if rawList[0].RefreshToken != "rt_sensitive_long_token_12345" {
		t.Errorf("expected raw refresh token unmasked in List()")
	}

	maskedList := store.ListMasked()
	if len(maskedList) != 1 {
		t.Fatalf("expected 1 masked record, got %d", len(maskedList))
	}
	m := maskedList[0]
	if m.RefreshToken == "rt_sensitive_long_token_12345" || !strings.Contains(m.RefreshToken, "...") {
		t.Errorf("expected masked RefreshToken, got %q", m.RefreshToken)
	}
	if m.AccessToken == "at_sensitive_access_token_67890" || !strings.Contains(m.AccessToken, "...") {
		t.Errorf("expected masked AccessToken, got %q", m.AccessToken)
	}
	if m.ClientSecret == "cs_secret_val" || !strings.Contains(m.ClientSecret, "...") {
		t.Errorf("expected masked ClientSecret, got %q", m.ClientSecret)
	}
}
