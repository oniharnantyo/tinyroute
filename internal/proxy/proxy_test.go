package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/cloudcode"
	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/credential"
	"github.com/oniharnantyo/tinyroute/internal/dialect"
	"github.com/oniharnantyo/tinyroute/internal/dialect/anthropic"
	"github.com/oniharnantyo/tinyroute/internal/dialect/gemini"
	"github.com/oniharnantyo/tinyroute/internal/dialect/openai"
	"github.com/oniharnantyo/tinyroute/internal/probe"
	"github.com/oniharnantyo/tinyroute/internal/proxy"
	_ "github.com/oniharnantyo/tinyroute/internal/translate/request"
	_ "github.com/oniharnantyo/tinyroute/internal/translate/response"
)

func TestJoinURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		relPath string
		want    string
	}{
		{
			name:    "base url with /v1 and relPath with /v1",
			baseURL: "https://opencode.ai/zen/v1",
			relPath: "/v1/chat/completions",
			want:    "https://opencode.ai/zen/v1/chat/completions",
		},
		{
			name:    "base url with /v1/ trailing slash and relPath with /v1",
			baseURL: "https://opencode.ai/zen/v1/",
			relPath: "/v1/chat/completions",
			want:    "https://opencode.ai/zen/v1/chat/completions",
		},
		{
			name:    "base url without /v1 and relPath with /v1",
			baseURL: "https://opencode.ai/zen",
			relPath: "/v1/chat/completions",
			want:    "https://opencode.ai/zen/v1/chat/completions",
		},
		{
			name:    "base url with /v1 and relPath without leading slash",
			baseURL: "https://api.openai.com/v1",
			relPath: "v1/chat/completions",
			want:    "https://api.openai.com/v1/chat/completions",
		},
		{
			name:    "base url anthropic without /v1 and relPath /v1/messages",
			baseURL: "https://api.anthropic.com",
			relPath: "/v1/messages",
			want:    "https://api.anthropic.com/v1/messages",
		},
		{
			name:    "base url anthropic with /v1 and relPath /v1/messages",
			baseURL: "https://api.anthropic.com/v1",
			relPath: "/v1/messages",
			want:    "https://api.anthropic.com/v1/messages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := proxy.JoinURL(tt.baseURL, tt.relPath)
			if got != tt.want {
				t.Errorf("JoinURL(%q, %q) = %q, want %q", tt.baseURL, tt.relPath, got, tt.want)
			}
		})
	}
}

type fakeRecorder struct {
	rec core.RequestRecord
	ch  chan core.RequestRecord
}

func newFakeRecorder() *fakeRecorder {
	return &fakeRecorder{ch: make(chan core.RequestRecord, 1)}
}

func (f *fakeRecorder) Record(ctx context.Context, rec core.RequestRecord) {
	f.ch <- rec
}

func TestHandler_CaptureEnrichment(t *testing.T) {
	rec := newFakeRecorder()

	// Mock server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"type":"message","usage":{"input_tokens":10,"output_tokens":5}}`))
	}))
	defer backend.Close()

	d := &anthropic.Dialect{}

	deps := &proxy.Deps{
		Transport: backend.Client().Transport.(*http.Transport),
		GetProvider: func(name string) (proxy.ProviderInfo, bool) {
			if name == "p1" {
				return proxy.ProviderInfo{Dialect: "anthropic", BaseURL: backend.URL}, true
			}
			return proxy.ProviderInfo{}, false
		},
		GetDialect: func(name string) (core.Dialect, bool) {
			if name == "anthropic" {
				return d, true
			}
			return nil, false
		},
		Health:      fakeHealth{},
		Selector:    fakeSelector{},
		Recorder:    rec,
		CaptureMode: "full",
	}

	handler := proxy.Handler(deps)

	reqCtx := &proxy.RequestCtx{
		RequestID: "req-test-1",
		KeyID:     "key-1",
		Dialect:   d,
		Parsed:    core.ParsedRequest{Model: "claude-3-5-sonnet", Stream: false},
		Route:     core.ResolvedRoute{Hops: []core.Hop{{Provider: "p1", Model: "claude-3-5-sonnet"}}},
	}

	r := httptest.NewRequest("POST", "/anthropic/v1/messages", bytes.NewReader([]byte(`{"model":"claude-3-5-sonnet","messages":[]}`)))
	r = r.WithContext(proxy.WithRequestContext(r.Context(), reqCtx))
	w := httptest.NewRecorder()

	handler(w, r)

	select {
	case recorded := <-rec.ch:
		if recorded.Provider != "p1" {
			t.Errorf("Provider = %q, want p1", recorded.Provider)
		}
		if recorded.Latency <= 0 {
			t.Errorf("Latency = %v, want > 0", recorded.Latency)
		}
		if recorded.RequestBody != `{"model":"claude-3-5-sonnet","messages":[]}` {
			t.Errorf("RequestBody = %q, want request json", recorded.RequestBody)
		}
		if recorded.ResponseBody != `{"type":"message","usage":{"input_tokens":10,"output_tokens":5}}` {
			t.Errorf("ResponseBody = %q, want response json", recorded.ResponseBody)
		}
		if recorded.TranslatedRequestBody == "" {
			t.Errorf("TranslatedRequestBody is empty, want translated request json")
		}
		if recorded.RawResponseBody != `{"type":"message","usage":{"input_tokens":10,"output_tokens":5}}` {
			t.Errorf("RawResponseBody = %q, want raw response json", recorded.RawResponseBody)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recorded record")
	}
}

func TestHandler_SSESavedAsJSON(t *testing.T) {
	rec := newFakeRecorder()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"id\":\"msg-1\"}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"text\":\"Hello\"}\n\ndata: [DONE]\n\n"))
	}))
	defer backend.Close()

	d := &anthropic.Dialect{}

	deps := &proxy.Deps{
		Transport: backend.Client().Transport.(*http.Transport),
		GetProvider: func(name string) (proxy.ProviderInfo, bool) {
			if name == "p1" {
				return proxy.ProviderInfo{Dialect: "anthropic", BaseURL: backend.URL}, true
			}
			return proxy.ProviderInfo{}, false
		},
		GetDialect: func(name string) (core.Dialect, bool) {
			if name == "anthropic" {
				return d, true
			}
			return nil, false
		},
		Health:      fakeHealth{},
		Selector:    fakeSelector{},
		Recorder:    rec,
		CaptureMode: "full",
	}

	handler := proxy.Handler(deps)

	reqCtx := &proxy.RequestCtx{
		RequestID: "req-sse-1",
		KeyID:     "key-1",
		Dialect:   d,
		Parsed:    core.ParsedRequest{Model: "claude-3-5-sonnet", Stream: true},
		Route:     core.ResolvedRoute{Hops: []core.Hop{{Provider: "p1", Model: "claude-3-5-sonnet"}}},
	}

	r := httptest.NewRequest("POST", "/anthropic/v1/messages", bytes.NewReader([]byte(`{"model":"claude-3-5-sonnet","messages":[],"stream":true}`)))
	r = r.WithContext(proxy.WithRequestContext(r.Context(), reqCtx))
	w := httptest.NewRecorder()

	handler(w, r)

	select {
	case recorded := <-rec.ch:
		wantJSON := `[{"type":"message_start","id":"msg-1"},{"type":"content_block_delta","text":"Hello"}]`
		if recorded.ResponseBody != wantJSON {
			t.Errorf("ResponseBody = %q, want JSON array %q", recorded.ResponseBody, wantJSON)
		}
		if recorded.RawResponseBody != wantJSON {
			t.Errorf("RawResponseBody = %q, want JSON array %q", recorded.RawResponseBody, wantJSON)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recorded record")
	}
}

func TestHandler_CredentialResolution(t *testing.T) {
	rec := newFakeRecorder()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer oauth-test-access-token" {
			t.Errorf("Authorization header = %q, want Bearer oauth-test-access-token", authHeader)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"type":"message","usage":{"input_tokens":5,"output_tokens":5}}`))
	}))
	defer backend.Close()

	d := &anthropic.Dialect{}
	oauthCred := credential.NewOAuthRefreshable(credential.OAuthRefreshableConfig{
		Provider:    "oauth-p",
		AccessToken: "oauth-test-access-token",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	})

	deps := &proxy.Deps{
		Transport: backend.Client().Transport.(*http.Transport),
		GetProvider: func(name string) (proxy.ProviderInfo, bool) {
			if name == "oauth-p" {
				return proxy.ProviderInfo{
					Dialect:    "anthropic",
					BaseURL:    backend.URL,
					Credential: oauthCred,
				}, true
			}
			return proxy.ProviderInfo{}, false
		},
		GetDialect: func(name string) (core.Dialect, bool) {
			if name == "anthropic" {
				return d, true
			}
			return nil, false
		},
		Health:      fakeHealth{},
		Selector:    fakeSelector{},
		Recorder:    rec,
		CaptureMode: "metadata",
	}

	handler := proxy.Handler(deps)
	reqCtx := &proxy.RequestCtx{
		RequestID: "req-cred-1",
		KeyID:     "key-1",
		Dialect:   d,
		Parsed:    core.ParsedRequest{Model: "claude-3-5-sonnet", Stream: false},
		Route:     core.ResolvedRoute{Hops: []core.Hop{{Provider: "oauth-p", Model: "claude-3-5-sonnet"}}},
	}

	r := httptest.NewRequest("POST", "/anthropic/v1/messages", bytes.NewReader([]byte(`{"model":"claude-3-5-sonnet","messages":[]}`)))
	r = r.WithContext(proxy.WithRequestContext(r.Context(), reqCtx))
	w := httptest.NewRecorder()

	handler(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200 OK", w.Code)
	}
}

type fakeHealth struct{}

func (fakeHealth) Available(provider string) bool                               { return true }
func (fakeHealth) AvailableModel(provider, model string) bool                   { return true }
func (fakeHealth) Penalize(provider string, duration time.Duration)             {}
func (fakeHealth) PenalizeModel(provider, model string, duration time.Duration) {}
func (fakeHealth) CooldownEnd(provider string) time.Time                        { return time.Time{} }
func (fakeHealth) Save() error                                                  { return nil }
func (fakeHealth) Load() error                                                  { return nil }

type fakeSelector struct{}

func (fakeSelector) Select(hops []core.Hop, available func(provider string) bool) []core.Hop {
	return hops
}

func TestHandler_CrossDialectTranslation(t *testing.T) {
	// Provider stub is an OpenAI dialect backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte(`"role":"system"`)) {
			t.Errorf("expected translated OpenAI request to contain role:system message, got %s", string(body))
		}

		if bytes.Contains(body, []byte(`"stream":true`)) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("data: {\"id\":\"chatcmpl-stream\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hello from OpenAI!\"}}]}\n\n"))
			w.Write([]byte("data: {\"id\":\"chatcmpl-stream\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":4}}\n\n"))
			w.Write([]byte("data: [DONE]\n\n"))
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":"chatcmpl-nonstream","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"Non-stream from OpenAI!"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":4}}`))
		}
	}))
	defer backend.Close()

	anthropicDialect := &anthropic.Dialect{}
	openAIDialect := &openai.Dialect{}

	deps := &proxy.Deps{
		Transport: backend.Client().Transport.(*http.Transport),
		GetProvider: func(name string) (proxy.ProviderInfo, bool) {
			if name == "openai-provider" {
				return proxy.ProviderInfo{Dialect: "openai", BaseURL: backend.URL}, true
			}
			return proxy.ProviderInfo{}, false
		},
		GetDialect: func(name string) (core.Dialect, bool) {
			if name == "anthropic" {
				return anthropicDialect, true
			}
			if name == "openai" {
				return openAIDialect, true
			}
			return nil, false
		},
		Health:   fakeHealth{},
		Selector: fakeSelector{},
		Recorder: newFakeRecorder(),
	}

	handler := proxy.Handler(deps)

	t.Run("non-streaming cross-dialect", func(t *testing.T) {
		reqCtx := &proxy.RequestCtx{
			RequestID: "req-cross-nonstream",
			KeyID:     "key-1",
			Dialect:   anthropicDialect,
			Parsed:    core.ParsedRequest{Model: "gpt-4o", Stream: false},
			Route:     core.ResolvedRoute{Hops: []core.Hop{{Provider: "openai-provider", Model: "gpt-4o"}}},
		}

		anthropicReq := `{"model":"gpt-4o","system":"You are a helpful bot.","messages":[{"role":"user","content":"Hi"}]}`
		r := httptest.NewRequest("POST", "/anthropic/v1/messages", bytes.NewReader([]byte(anthropicReq)))
		r = r.WithContext(proxy.WithRequestContext(r.Context(), reqCtx))
		w := httptest.NewRecorder()

		handler(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("StatusCode = %d, want 200 OK", w.Code)
		}

		respStr := w.Body.String()
		if !bytes.Contains(w.Body.Bytes(), []byte(`"type":"message"`)) || !bytes.Contains(w.Body.Bytes(), []byte("Non-stream from OpenAI!")) {
			t.Errorf("unexpected non-streaming response body: %s", respStr)
		}
	})

	t.Run("streaming cross-dialect", func(t *testing.T) {
		reqCtx := &proxy.RequestCtx{
			RequestID: "req-cross-stream",
			KeyID:     "key-1",
			Dialect:   anthropicDialect,
			Parsed:    core.ParsedRequest{Model: "gpt-4o", Stream: true},
			Route:     core.ResolvedRoute{Hops: []core.Hop{{Provider: "openai-provider", Model: "gpt-4o"}}},
		}

		anthropicReq := `{"model":"gpt-4o","system":"You are a helpful bot.","messages":[{"role":"user","content":"Hi"}],"stream":true}`
		r := httptest.NewRequest("POST", "/anthropic/v1/messages", bytes.NewReader([]byte(anthropicReq)))
		r = r.WithContext(proxy.WithRequestContext(r.Context(), reqCtx))
		w := httptest.NewRecorder()

		handler(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("StatusCode = %d, want 200 OK", w.Code)
		}

		respStr := w.Body.String()
		if !bytes.Contains(w.Body.Bytes(), []byte("event: message_start")) || !bytes.Contains(w.Body.Bytes(), []byte("Hello from OpenAI!")) {
			t.Errorf("unexpected streaming response body: %s", respStr)
		}
	})
}

func TestHandler_TwoHopTranslation_AnthropicToGemini(t *testing.T) {
	// Provider is a Gemini dialect backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte(`"systemInstruction"`)) || !bytes.Contains(body, []byte(`"contents"`)) {
			t.Errorf("expected translated Gemini payload containing systemInstruction and contents, got %s", string(body))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"candidates": [
				{
					"index": 0,
					"content": {
						"parts": [{"text": "Hello from Gemini via two-hop!"}],
						"role": "model"
					},
					"finishReason": "STOP"
				}
			],
			"usageMetadata": {
				"promptTokenCount": 20,
				"candidatesTokenCount": 10,
				"totalTokenCount": 30
			}
		}`))
	}))
	defer backend.Close()

	anthropicDialect := &anthropic.Dialect{}
	openAIDialect := &openai.Dialect{}
	geminiDialect := &gemini.Dialect{}

	deps := &proxy.Deps{
		Transport: backend.Client().Transport.(*http.Transport),
		GetProvider: func(name string) (proxy.ProviderInfo, bool) {
			if name == "gemini-provider" {
				return proxy.ProviderInfo{Dialect: "gemini", BaseURL: backend.URL}, true
			}
			return proxy.ProviderInfo{}, false
		},
		GetDialect: func(name string) (core.Dialect, bool) {
			if name == "anthropic" {
				return anthropicDialect, true
			}
			if name == "openai" {
				return openAIDialect, true
			}
			if name == "gemini" {
				return geminiDialect, true
			}
			return nil, false
		},
		Health:   fakeHealth{},
		Selector: fakeSelector{},
		Recorder: newFakeRecorder(),
	}

	handler := proxy.Handler(deps)

	reqCtx := &proxy.RequestCtx{
		RequestID: "req-twohop-1",
		KeyID:     "key-1",
		Dialect:   anthropicDialect,
		Parsed:    core.ParsedRequest{Model: "gemini-1.5-pro", Stream: false},
		Route:     core.ResolvedRoute{Hops: []core.Hop{{Provider: "gemini-provider", Model: "gemini-1.5-pro"}}},
	}

	anthropicReq := `{"model":"gemini-1.5-pro","system":"You are a helpful assistant.","messages":[{"role":"user","content":"Hello"}]}`
	r := httptest.NewRequest("POST", "/anthropic/v1/messages", bytes.NewReader([]byte(anthropicReq)))
	r = r.WithContext(proxy.WithRequestContext(r.Context(), reqCtx))
	w := httptest.NewRecorder()

	handler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("StatusCode = %d, want 200 OK", w.Code)
	}

	if !bytes.Contains(w.Body.Bytes(), []byte(`"type":"message"`)) || !bytes.Contains(w.Body.Bytes(), []byte("Hello from Gemini via two-hop!")) {
		t.Errorf("unexpected two-hop response body: %s", w.Body.String())
	}
}

type fakeCred struct{ err error }

func (f fakeCred) Token(ctx context.Context) (credential.TokenResult, error) {
	if f.err != nil {
		return credential.TokenResult{}, f.err
	}
	return credential.TokenResult{Value: "tok-123"}, nil
}

func TestHandler_ErrorCases(t *testing.T) {
	deps := &proxy.Deps{
		GetProvider: func(name string) (proxy.ProviderInfo, bool) { return proxy.ProviderInfo{}, false },
		GetDialect:  func(name string) (core.Dialect, bool) { return nil, false },
		Health:      fakeHealth{},
		Selector:    fakeSelector{},
		Recorder:    newFakeRecorder(),
	}

	handler := proxy.Handler(deps)

	t.Run("missing request context", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte("{}")))
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected 500 for missing context, got %d", w.Code)
		}
	})

	t.Run("body exceeds size limit", func(t *testing.T) {
		anthropicDialect := &anthropic.Dialect{}
		reqCtx := &proxy.RequestCtx{
			RequestID: "req-big",
			Dialect:   anthropicDialect,
		}
		// 33 MB payload > maxBodySize (32 MB)
		bigData := make([]byte, 33<<20)
		r := httptest.NewRequest("POST", "/anthropic/v1/messages", bytes.NewReader(bigData))
		r = r.WithContext(proxy.WithRequestContext(r.Context(), reqCtx))
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("expected 413 for oversized body, got %d", w.Code)
		}
	})

	t.Run("translate request failure skips hop", func(t *testing.T) {
		anthropicDialect := &anthropic.Dialect{}
		openAIDialect := &openai.Dialect{}
		deps := &proxy.Deps{
			GetProvider: func(name string) (proxy.ProviderInfo, bool) {
				return proxy.ProviderInfo{Dialect: "openai", BaseURL: "http://localhost"}, true
			},
			GetDialect: func(name string) (core.Dialect, bool) {
				if name == "anthropic" {
					return anthropicDialect, true
				}
				return openAIDialect, true
			},
			Health:   fakeHealth{},
			Selector: fakeSelector{},
			Recorder: newFakeRecorder(),
		}
		handler := proxy.Handler(deps)
		reqCtx := &proxy.RequestCtx{
			RequestID: "req-err",
			Dialect:   anthropicDialect,
			Route:     core.ResolvedRoute{Hops: []core.Hop{{Provider: "openai-provider", Model: "gpt-4o"}}},
		}
		r := httptest.NewRequest("POST", "/anthropic/v1/messages", bytes.NewReader([]byte("not-json")))
		r = r.WithContext(proxy.WithRequestContext(r.Context(), reqCtx))
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusBadGateway {
			t.Errorf("expected 502 when translation fails, got %d", w.Code)
		}
	})

	t.Run("credential token error", func(t *testing.T) {
		anthropicDialect := &anthropic.Dialect{}
		openAIDialect := &openai.Dialect{}
		errCred := fakeCred{err: fmt.Errorf("token error")}
		deps := &proxy.Deps{
			GetProvider: func(name string) (proxy.ProviderInfo, bool) {
				return proxy.ProviderInfo{Dialect: "openai", BaseURL: "http://localhost", Credential: errCred}, true
			},
			GetDialect: func(name string) (core.Dialect, bool) {
				if name == "anthropic" {
					return anthropicDialect, true
				}
				return openAIDialect, true
			},
			Health:   fakeHealth{},
			Selector: fakeSelector{},
			Recorder: newFakeRecorder(),
		}
		handler := proxy.Handler(deps)
		reqCtx := &proxy.RequestCtx{
			RequestID: "req-tok-err",
			Dialect:   anthropicDialect,
			Route:     core.ResolvedRoute{Hops: []core.Hop{{Provider: "openai-provider", Model: "gpt-4o"}}},
		}
		r := httptest.NewRequest("POST", "/anthropic/v1/messages", bytes.NewReader([]byte(`{"messages":[]}`)))
		r = r.WithContext(proxy.WithRequestContext(r.Context(), reqCtx))
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusBadGateway {
			t.Errorf("expected 502 when token fails, got %d", w.Code)
		}
	})

	t.Run("upstream network error failover", func(t *testing.T) {
		anthropicDialect := &anthropic.Dialect{}
		openAIDialect := &openai.Dialect{}
		deps := &proxy.Deps{
			Transport: &http.Transport{},
			GetProvider: func(name string) (proxy.ProviderInfo, bool) {
				return proxy.ProviderInfo{Dialect: "openai", BaseURL: "http://127.0.0.1:59999"}, true
			},
			GetDialect: func(name string) (core.Dialect, bool) {
				if name == "anthropic" {
					return anthropicDialect, true
				}
				return openAIDialect, true
			},
			Health:      fakeHealth{},
			Selector:    fakeSelector{},
			Recorder:    newFakeRecorder(),
			Cooldown5xx: 1 * time.Second,
		}
		handler := proxy.Handler(deps)
		reqCtx := &proxy.RequestCtx{
			RequestID: "req-net-err",
			Dialect:   anthropicDialect,
			Route:     core.ResolvedRoute{Hops: []core.Hop{{Provider: "openai-provider", Model: "gpt-4o"}}},
		}
		r := httptest.NewRequest("POST", "/anthropic/v1/messages", bytes.NewReader([]byte(`{"messages":[]}`)))
		r = r.WithContext(proxy.WithRequestContext(r.Context(), reqCtx))
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusBadGateway {
			t.Errorf("expected 502 when network fails, got %d", w.Code)
		}
	})
}

// recordingHealth counts PenalizeModel calls so tests can assert whether a
// failure cooled a provider down. Penalties are applied synchronously in the
// attempt loop, so no locking is needed when read after Handler returns.
type recordingHealth struct {
	fakeHealth
	penaltyN int
}

func (h *recordingHealth) PenalizeModel(provider, model string, _ time.Duration) {
	h.penaltyN++
}

// TestNoPenaltiesSuppressesCooldown asserts that a probe-scoped Deps
// (NoPenalties=true) does not cool a provider down when a hop fails, while a
// normal Deps still does.
func TestNoPenaltiesSuppressesCooldown(t *testing.T) {
	anthropicDialect := &anthropic.Dialect{}
	openAIDialect := &openai.Dialect{}

	doProbe := func(noPenalties bool) int {
		health := &recordingHealth{}
		deps := &proxy.Deps{
			Transport: &http.Transport{},
			GetProvider: func(name string) (proxy.ProviderInfo, bool) {
				return proxy.ProviderInfo{Dialect: "openai", BaseURL: "http://127.0.0.1:59999"}, true
			},
			GetDialect: func(name string) (core.Dialect, bool) {
				if name == "anthropic" {
					return anthropicDialect, true
				}
				return openAIDialect, true
			},
			Health:      health,
			Selector:    fakeSelector{},
			Recorder:    newFakeRecorder(),
			Cooldown5xx: 1 * time.Second,
			NoPenalties: noPenalties,
		}
		handler := proxy.Handler(deps)
		reqCtx := &proxy.RequestCtx{
			Dialect:   anthropicDialect,
			Parsed:    core.ParsedRequest{Model: "gpt-4o"},
			Route:     core.ResolvedRoute{Hops: []core.Hop{{Provider: "p1", Model: "gpt-4o"}}},
			RequestID: "req-1",
		}
		r := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader([]byte(`{"messages":[{"role":"user","content":"hi"}]}`)))
		r = r.WithContext(proxy.WithRequestContext(r.Context(), reqCtx))
		w := httptest.NewRecorder()
		handler(w, r)
		return health.penaltyN
	}

	if n := doProbe(false); n == 0 {
		t.Errorf("expected penalties when NoPenalties is false, got 0")
	}
	if n := doProbe(true); n != 0 {
		t.Errorf("expected no penalties when NoPenalties is true, got %d", n)
	}
}

func TestCloudCodeTransportHook(t *testing.T) {
	geminiDialect, _ := dialect.ByName("gemini")
	var capturedURL, capturedAuth, capturedUA string
	var capturedEnvelope map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		capturedAuth = r.Header.Get("Authorization")
		capturedUA = r.Header.Get("User-Agent")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedEnvelope)

		if r.URL.Path == "/v1internal:loadCodeAssist" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"cloudaicompanionProject": "mock-proj-1",
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"cloudcode response"}]}}]}`))
	}))
	defer server.Close()

	onboarding := cloudcode.NewOnboarding(server.URL, server.Client())
	deps := &proxy.Deps{
		Transport: server.Client().Transport.(*http.Transport),
		GetProvider: func(name string) (proxy.ProviderInfo, bool) {
			if name == "antigravity" {
				return proxy.ProviderInfo{
					Dialect:   "gemini",
					BaseURL:   server.URL,
					Transport: "cloudcode",
					APIKey:    "test-oauth-token",
				}, true
			}
			return proxy.ProviderInfo{}, false
		},
		GetDialect: func(name string) (core.Dialect, bool) {
			return geminiDialect, true
		},
		CloudCodeOnboarding: onboarding,
		Health:              fakeHealth{},
		Selector:            fakeSelector{},
		Recorder:            newFakeRecorder(),
	}

	handler := proxy.Handler(deps)
	reqCtx := &proxy.RequestCtx{
		Dialect:   geminiDialect,
		Parsed:    core.ParsedRequest{Model: "gemini-3.6-flash-medium"},
		Route:     core.ResolvedRoute{Hops: []core.Hop{{Provider: "antigravity", Model: "gemini-3.6-flash-medium"}}},
		RequestID: "req-cc-1",
	}

	nativeBody := []byte(`{"contents":[{"parts":[{"text":"hello cloudcode"}]}]}`)
	r := httptest.NewRequest("POST", "/v1beta/models/gemini-3.6-flash-medium:generateContent", bytes.NewReader(nativeBody))
	r = r.WithContext(proxy.WithRequestContext(r.Context(), reqCtx))
	w := httptest.NewRecorder()

	handler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	if capturedURL != "/v1internal:generateContent" {
		t.Errorf("got URL %q, want /v1internal:generateContent", capturedURL)
	}
	if capturedAuth != "Bearer test-oauth-token" {
		t.Errorf("got Authorization %q, want Bearer test-oauth-token", capturedAuth)
	}
	if capturedUA != cloudcode.DefaultUserAgent {
		t.Errorf("got User-Agent %q, want %q", capturedUA, cloudcode.DefaultUserAgent)
	}
	if capturedEnvelope["project"] != "mock-proj-1" {
		t.Errorf("got project %v, want mock-proj-1", capturedEnvelope["project"])
	}
	if capturedEnvelope["model"] != "gemini-3.6-flash-medium" {
		t.Errorf("got model %v, want gemini-3.6-flash-medium", capturedEnvelope["model"])
	}
}

func TestRunInProcess_CloudCode(t *testing.T) {
	geminiDialect, _ := dialect.ByName("gemini")
	var capturedURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		if r.URL.Path == "/v1internal:loadCodeAssist" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"cloudaicompanionProject": "probe-proj-1",
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"probe response"}]}}]}`))
	}))
	defer server.Close()

	onboarding := cloudcode.NewOnboarding(server.URL, server.Client())
	deps := &proxy.Deps{
		Transport: server.Client().Transport.(*http.Transport),
		GetProvider: func(name string) (proxy.ProviderInfo, bool) {
			if name == "antigravity" {
				return proxy.ProviderInfo{
					Dialect:   "gemini",
					BaseURL:   server.URL,
					Transport: "cloudcode",
					APIKey:    "probe-token",
				}, true
			}
			return proxy.ProviderInfo{}, false
		},
		GetDialect: func(name string) (core.Dialect, bool) {
			return geminiDialect, true
		},
		CloudCodeOnboarding: onboarding,
		Health:              fakeHealth{},
		Selector:            fakeSelector{},
		Recorder:            newFakeRecorder(),
	}

	handler := proxy.Handler(deps)
	resolve := func(dialectName, model string) (core.ResolvedRoute, error) {
		return core.ResolvedRoute{Hops: []core.Hop{{Provider: "antigravity", Model: "gemini-3.6-flash-medium"}}}, nil
	}

	code, _, err := probe.RunInProcess(
		context.Background(),
		"antigravity",
		"gemini",
		"gemini-3.6-flash-medium",
		resolve,
		handler,
		5*time.Second,
	)
	if err != nil {
		t.Fatalf("unexpected probe error: %v", err)
	}
	if code != http.StatusOK {
		t.Errorf("got probe status %d, want 200", code)
	}
	if capturedURL != "/v1internal:generateContent" {
		t.Errorf("got endpoint %q, want /v1internal:generateContent", capturedURL)
	}
}
