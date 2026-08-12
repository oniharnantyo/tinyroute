package proxy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/dialect/openai"
	"github.com/oniharnantyo/tinyroute/internal/proxy"
)

func TestDefaultFusionRunner_PoolAndFused(t *testing.T) {
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"response-1"}}]}`))
	}))
	defer ts1.Close()

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"response-2"}}]}`))
	}))
	defer ts2.Close()

	deps := &proxy.Deps{
		Transport: http.DefaultTransport.(*http.Transport),
		GetProvider: func(name string) (proxy.ProviderInfo, bool) {
			if name == "p1" {
				return proxy.ProviderInfo{Dialect: "openai", BaseURL: ts1.URL, APIKey: "sk-1"}, true
			}
			if name == "p2" {
				return proxy.ProviderInfo{Dialect: "openai", BaseURL: ts2.URL, APIKey: "sk-2"}, true
			}
			return proxy.ProviderInfo{}, false
		},
		GetDialect: func(name string) (core.Dialect, bool) {
			if name == "openai" {
				return &openai.Dialect{}, true
			}
			return nil, false
		},
	}

	runner := &proxy.DefaultFusionRunner{Deps: deps}

	hops := []core.Hop{
		{Provider: "p1", Model: "gpt-4o"},
		{Provider: "p2", Model: "gpt-4o"},
	}

	// Test RunPool
	poolRes, err := runner.RunPool(context.Background(), hops, []byte(`{"model":"gpt-4o"}`))
	if err != nil {
		t.Fatalf("unexpected pool error: %v", err)
	}
	if poolRes.Outcome != core.OutcomeOK {
		t.Errorf("expected OutcomeOK, got %s", poolRes.Outcome)
	}

	// Test RunFused
	fusedRes, err := runner.RunFused(context.Background(), hops, []byte(`{"model":"gpt-4o"}`))
	if err != nil {
		t.Fatalf("unexpected fused error: %v", err)
	}
	if fusedRes.Outcome != core.OutcomeOK {
		t.Errorf("expected OutcomeOK, got %s", fusedRes.Outcome)
	}
}
