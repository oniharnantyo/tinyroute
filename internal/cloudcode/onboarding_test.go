package cloudcode_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/cloudcode"
)

func TestOnboarding_LoadCodeAssist_Success(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		if r.URL.Path != "/v1internal:loadCodeAssist" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-access-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("User-Agent") != cloudcode.DefaultUserAgent {
			http.Error(w, "bad user-agent", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"cloudaicompanionProject": "project-12345",
		})
	}))
	defer server.Close()

	onboarding := cloudcode.NewOnboarding(server.URL, server.Client())
	projectID, err := onboarding.ProjectID(context.Background(), "test-access-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if projectID != "project-12345" {
		t.Errorf("got projectID %q, want project-12345", projectID)
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 HTTP call, got %d", callCount)
	}

	// Second call should hit cache and NOT make HTTP request
	projectID2, err := onboarding.ProjectID(context.Background(), "test-access-token")
	if err != nil {
		t.Fatalf("unexpected error on cached call: %v", err)
	}
	if projectID2 != "project-12345" {
		t.Errorf("got cached projectID %q, want project-12345", projectID2)
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected still 1 HTTP call after cache hit, got %d", callCount)
	}
}

func TestOnboarding_ObjectProjectID_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return project as an object `{"id": "project-obj-999"}`
		_, _ = w.Write([]byte(`{"cloudaicompanionProject": {"id": "project-obj-999"}}`))
	}))
	defer server.Close()

	onboarding := cloudcode.NewOnboarding(server.URL, server.Client())
	projectID, err := onboarding.ProjectID(context.Background(), "obj-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if projectID != "project-obj-999" {
		t.Errorf("got projectID %q, want project-obj-999", projectID)
	}
}

func TestOnboarding_LoadCodeAssistFallbackToOnboardUser(t *testing.T) {
	var loadCalls, onboardCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1internal:loadCodeAssist":
			atomic.AddInt32(&loadCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			// Return empty project ID to trigger fallback
			_ = json.NewEncoder(w).Encode(map[string]string{
				"cloudaicompanionProject": "",
			})
		case "/v1internal:onboardUser":
			atomic.AddInt32(&onboardCalls, 1)
			if r.Header.Get("Authorization") != "Bearer fallback-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"cloudaicompanionProject": "fallback-project-67890",
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	onboarding := cloudcode.NewOnboarding(server.URL, server.Client())
	projectID, err := onboarding.ProjectID(context.Background(), "fallback-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if projectID != "fallback-project-67890" {
		t.Errorf("got projectID %q, want fallback-project-67890", projectID)
	}
	if atomic.LoadInt32(&loadCalls) != 1 {
		t.Errorf("expected 1 loadCodeAssist call, got %d", loadCalls)
	}
	if atomic.LoadInt32(&onboardCalls) != 1 {
		t.Errorf("expected 1 onboardUser call, got %d", onboardCalls)
	}
}

func TestOnboarding_CacheExpiry(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"cloudaicompanionProject": "proj-exp",
		})
	}))
	defer server.Close()

	now := time.Now()
	onboarding := cloudcode.NewOnboarding(server.URL, server.Client())
	onboarding.CacheTTL = 10 * time.Minute
	onboarding.Now = func() time.Time { return now }

	// First call
	_, err := onboarding.ProjectID(context.Background(), "exp-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Advance time by 5 minutes -> still cached
	now = now.Add(5 * time.Minute)
	_, _ = onboarding.ProjectID(context.Background(), "exp-token")
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Advance time past TTL (11 minutes) -> cache expired, new call made
	now = now.Add(6 * time.Minute)
	_, _ = onboarding.ProjectID(context.Background(), "exp-token")
	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected 2 calls after expiry, got %d", callCount)
	}
}

func TestOnboarding_ConcurrentRequestsDeduplication(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		time.Sleep(50 * time.Millisecond) // simulate latency
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"cloudaicompanionProject": "concurrent-proj",
		})
	}))
	defer server.Close()

	onboarding := cloudcode.NewOnboarding(server.URL, server.Client())
	var wg sync.WaitGroup
	const numGoroutines = 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pid, err := onboarding.ProjectID(context.Background(), "same-token")
			if err != nil {
				t.Errorf("concurrent error: %v", err)
			}
			if pid != "concurrent-proj" {
				t.Errorf("got pid %q, want concurrent-proj", pid)
			}
		}()
	}

	wg.Wait()
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 HTTP call for concurrent requests, got %d", callCount)
	}
}

func TestOnboarding_EmptyAccessToken_ReturnsError(t *testing.T) {
	onboarding := cloudcode.NewOnboarding("", nil)
	_, err := onboarding.ProjectID(context.Background(), "")
	if err == nil {
		t.Errorf("expected error for empty access token, got nil")
	}
}

func TestOnboarding_BothEndpointsFail_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	onboarding := cloudcode.NewOnboarding(server.URL, server.Client())
	_, err := onboarding.ProjectID(context.Background(), "error-token")
	if err == nil {
		t.Errorf("expected error when both loadCodeAssist and onboardUser fail, got nil")
	}
}
