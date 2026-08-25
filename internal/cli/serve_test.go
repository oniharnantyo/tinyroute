package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/dialect"
	_ "github.com/oniharnantyo/tinyroute/internal/dialect/anthropic"
	_ "github.com/oniharnantyo/tinyroute/internal/dialect/gemini"
	_ "github.com/oniharnantyo/tinyroute/internal/dialect/openai"
	_ "github.com/oniharnantyo/tinyroute/internal/dialect/openairesponses"
	"github.com/oniharnantyo/tinyroute/internal/route"
)

func TestNamespacedSurfacesAndLegacy404(t *testing.T) {
	providers := map[string]config.Provider{
		"openai": {
			Dialect: "openai",
			BaseURL: "https://api.openai.com/v1",
			Models:  []string{"gpt-4o"},
		},
		"anthropic": {
			Dialect: "anthropic",
			BaseURL: "https://api.anthropic.com",
			Models:  []string{"claude-3-5-sonnet"},
		},
	}
	r := route.New(providers)

	mux := http.NewServeMux()
	for _, name := range dialect.Names() {
		d, ok := dialect.ByName(name)
		if !ok {
			continue
		}
		for _, p := range d.MountPaths() {
			mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
		}
		modelsMount := d.ModelsMountPath()
		if modelsMount == "" {
			continue
		}
		dCopy := d
		modelsPath := "GET " + modelsMount
		mux.HandleFunc(modelsPath, func(w http.ResponseWriter, req *http.Request) {
			dCopy.WriteModels(w, r.Models(dCopy.Name()))
		})
	}

	// 1. OpenAI models
	reqOpenAI := httptest.NewRequest("GET", "/openai/v1/models", nil)
	recOpenAI := httptest.NewRecorder()
	mux.ServeHTTP(recOpenAI, reqOpenAI)
	if recOpenAI.Code != http.StatusOK {
		t.Errorf("GET /openai/v1/models status = %d, want 200", recOpenAI.Code)
	}

	// 2. Anthropic models
	reqAnth := httptest.NewRequest("GET", "/anthropic/v1/models", nil)
	recAnth := httptest.NewRecorder()
	mux.ServeHTTP(recAnth, reqAnth)
	if recAnth.Code != http.StatusOK {
		t.Errorf("GET /anthropic/v1/models status = %d, want 200", recAnth.Code)
	}

	var anthResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recAnth.Body.Bytes(), &anthResp); err != nil {
		t.Fatalf("unmarshal anthropic models response failed: %v", err)
	}
	for _, m := range anthResp.Data {
		if _, err := r.Resolve("anthropic", m.ID); err != nil {
			t.Errorf("Anthropic model %q failed to resolve: %v", m.ID, err)
		}
	}

	// 3. Method restriction on /openai/v1/models
	reqPost := httptest.NewRequest("POST", "/openai/v1/models", nil)
	recPost := httptest.NewRecorder()
	mux.ServeHTTP(recPost, reqPost)
	if recPost.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /openai/v1/models status = %d, want 405", recPost.Code)
	}

	// 4. Legacy endpoints return 404
	legacyPaths := []string{
		"/v1/chat/completions",
		"/v1/messages",
		"/v1/models",
	}
	for _, path := range legacyPaths {
		req := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("Legacy path %s status = %d, want 404", path, rec.Code)
		}
	}

	// 5. The openai-responses dialect does not register its own models endpoint;
	//    it shares /openai/v1/models, so /openai-responses/v1/models is gone.
	respReq := httptest.NewRequest("GET", "/openai-responses/v1/models", nil)
	respRec := httptest.NewRecorder()
	mux.ServeHTTP(respRec, respReq)
	if respRec.Code != http.StatusNotFound {
		t.Errorf("GET /openai-responses/v1/models status = %d, want 404 (responses shares /openai/v1/models)", respRec.Code)
	}
}

// TestProviderInfoPropagatesCloudCodeTransport is the regression test for the
// antigravity model-test 404: a hand-edited antigravity entry omits transport,
// ParseTopology migrates it to transport=cloudcode, and providerInfo must carry
// that into proxy.ProviderInfo — otherwise proxy.Handler's cloudcode branch is
// never taken and the probe hits /v1beta/models (HTML 404).
func TestProviderInfoPropagatesCloudCodeTransport(t *testing.T) {
	raw := `providers:
  antigravity:
    dialect: gemini
    base_url: https://generativelanguage.googleapis.com`
	topo, err := config.ParseTopology([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p := topo.Providers["antigravity"]
	if p.Transport != "cloudcode" {
		t.Fatalf("migration did not set transport: got %q", p.Transport)
	}

	info := providerInfo("antigravity", p, nil)
	if info.Transport != "cloudcode" {
		t.Errorf("providerInfo dropped Transport: got %q, want %q", info.Transport, "cloudcode")
	}
	if info.BaseURL != "https://daily-cloudcode-pa.googleapis.com" {
		t.Errorf("providerInfo dropped migrated BaseURL: got %q", info.BaseURL)
	}
}
