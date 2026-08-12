package probe

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/core"
	_ "github.com/oniharnantyo/tinyroute/internal/dialect/openai"
)

func TestProbeBodyFor(t *testing.T) {
	anthropicBody := ProbeBodyFor("anthropic")
	if anthropicBody == "" || !testing.Short() && len(anthropicBody) < 10 {
		t.Errorf("unexpected anthropic probe body: %s", anthropicBody)
	}

	defaultBody := ProbeBodyFor("openai")
	if defaultBody == "" {
		t.Errorf("unexpected openai probe body: %s", defaultBody)
	}

	// Gemini's generateContent uses "contents", not the OpenAI/Anthropic
	// "messages". A probe body with "messages" is rejected by the CloudCode
	// backend with INVALID_ARGUMENT "Unknown name \"messages\"".
	geminiBody := ProbeBodyFor("gemini")
	if strings.Contains(geminiBody, "messages") {
		t.Errorf("gemini probe body must use 'contents', not 'messages': %s", geminiBody)
	}
	if !strings.Contains(geminiBody, "contents") {
		t.Errorf("gemini probe body missing 'contents': %s", geminiBody)
	}
}

func TestTestModelSuccessAndFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" && r.Header.Get("x-api-key") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"chatcmpl-test"}`))
	}))
	defer ts.Close()

	prov := config.Provider{
		Dialect: "openai",
		BaseURL: ts.URL,
		APIKey:  "sk-test-key",
	}

	ctx := context.Background()
	status, elapsed, err := TestModel(ctx, "openai", prov, "gpt-4o", "", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error probing test server: %v", err)
	}

	if status != http.StatusOK {
		t.Errorf("expected status 200, got %d", status)
	}

	if elapsed <= 0 {
		t.Errorf("expected positive elapsed duration")
	}

	// Unknown dialect error case
	invalidProv := config.Provider{Dialect: "unknown_dialect"}
	_, _, err = TestModel(ctx, "invalid", invalidProv, "model", "", 1*time.Second)
	if err == nil {
		t.Errorf("expected error for unknown dialect")
	}
}

func TestRequestBody(t *testing.T) {
	body, err := RequestBody("openai", "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(body, []byte("gpt-4o")) {
		t.Errorf("expected body to contain the target model, got: %s", body)
	}
	if bytes.Contains(body, []byte("probe")) {
		t.Errorf("expected placeholder model %q to be rewritten, got: %s", "probe", body)
	}

	if _, err := RequestBody("unknown_dialect", "m"); err == nil {
		t.Errorf("expected error for unknown dialect")
	}
}

func TestRunInProcess(t *testing.T) {
	var gotModel string
	resolve := func(name, model string) (core.ResolvedRoute, error) {
		gotModel = model
		if name != "openai" || model != "openai:gpt-4o" {
			return core.ResolvedRoute{}, fmt.Errorf("no route for %s:%s", name, model)
		}
		return core.ResolvedRoute{Hops: []core.Hop{{Provider: "openai", Model: "gpt-4o"}}}, nil
	}

	t.Run("happy path drives handler and returns its status", func(t *testing.T) {
		called := false
		handler := func(w http.ResponseWriter, r *http.Request) {
			called = true
			// A real proxy.Handler would 500 here without a RequestCtx attached;
			// this fake stands in for it but the call proves the request was built.
			// A non-empty body is required so the empty-response guard passes.
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":"probe","choices":[{"message":{"content":"pong"}}]}`))
		}
		code, elapsed, err := RunInProcess(context.Background(), "openai", "openai", "gpt-4o", resolve, handler, 5*time.Second)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotModel != "openai:gpt-4o" {
			t.Errorf("expected resolve to receive prefixed model %q, got %q", "openai:gpt-4o", gotModel)
		}
		if !called {
			t.Errorf("expected handler to be invoked")
		}
		if code != http.StatusOK {
			t.Errorf("expected status 200, got %d", code)
		}
		if elapsed <= 0 {
			t.Errorf("expected positive elapsed duration")
		}
	})

	t.Run("unknown dialect", func(t *testing.T) {
		_, _, err := RunInProcess(context.Background(), "openai", "nope", "m", resolve, func(http.ResponseWriter, *http.Request) {}, time.Second)
		if err == nil {
			t.Errorf("expected error for unknown dialect")
		}
	})

	t.Run("unroutable model", func(t *testing.T) {
		_, _, err := RunInProcess(context.Background(), "openai", "openai", "missing", resolve, func(http.ResponseWriter, *http.Request) {}, time.Second)
		if err == nil {
			t.Errorf("expected error for unroutable model")
		}
	})

	t.Run("2xx with empty body is a false-positive failure", func(t *testing.T) {
		emptyHandler := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}
		_, _, err := RunInProcess(context.Background(), "openai", "openai", "gpt-4o", resolve, emptyHandler, time.Second)
		if err == nil {
			t.Errorf("expected error for empty response body on 2xx")
		}
	})
}
