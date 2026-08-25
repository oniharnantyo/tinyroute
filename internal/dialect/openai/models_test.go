package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/route"
)

func createModelsMux(getRouter func() (*route.Router, error)) http.Handler {
	d := &Dialect{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /openai/v1/models", func(w http.ResponseWriter, r *http.Request) {
		router, err := getRouter()
		if err != nil {
			d.WriteError(w, http.StatusInternalServerError, "api_error", err.Error())
			return
		}
		WriteModelsResponse(w, router.Models(d.Name()))
	})
	return mux
}

func TestModelsEndpoint_ModelListingAndResolution(t *testing.T) {
	providers := map[string]config.Provider{
		"openai": {
			Dialect: "openai",
			BaseURL: "https://api.openai.com/v1",
			Models:  []string{"gpt-4o"},
		},
	}
	combos := []config.Combo{
		{
			Name:    "fast",
			Members: []string{"openai:gpt-4o"},
		},
	}
	r := route.New(providers, route.WithCombos(combos))

	mux := createModelsMux(func() (*route.Router, error) {
		return r, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp ModelsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}

	ids := make(map[string]bool)
	for _, entry := range resp.Data {
		ids[entry.ID] = true
	}

	if !ids["openai:gpt-4o"] {
		t.Errorf("expected openai:gpt-4o to be present in returned models")
	}
	if !ids["combo:fast"] {
		t.Errorf("expected combo:fast to be present in returned models")
	}
	if ids["fast"] {
		t.Errorf("expected bare combo name 'fast' to be absent from returned models")
	}
	if ids["gpt-4o"] {
		t.Errorf("expected bare gpt-4o to be absent from returned models")
	}

	for _, entry := range resp.Data {
		if _, err := r.Resolve("openai", entry.ID); err != nil {
			t.Errorf("returned model id %q failed to resolve: %v", entry.ID, err)
		}
	}
}

func TestModelsEndpoint_StableFields(t *testing.T) {
	providers := map[string]config.Provider{
		"openai": {
			Dialect: "openai",
			BaseURL: "https://api.openai.com/v1",
			Models:  []string{"gpt-4o"},
		},
	}
	r := route.New(providers)
	mux := createModelsMux(func() (*route.Router, error) {
		return r, nil
	})

	for reqNum := 1; reqNum <= 2; reqNum++ {
		req := httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", reqNum, rec.Code, http.StatusOK)
		}

		var resp ModelsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("request %d: unmarshal response failed: %v", reqNum, err)
		}

		for _, entry := range resp.Data {
			if entry.Created != 0 {
				t.Errorf("request %d, model %q: created = %d, want 0", reqNum, entry.ID, entry.Created)
			}
			if entry.OwnedBy != "tinyroute" {
				t.Errorf("request %d, model %q: owned_by = %q, want \"tinyroute\"", reqNum, entry.ID, entry.OwnedBy)
			}
		}
	}
}

func TestModelsEndpoint_MethodNotAllowed(t *testing.T) {
	providers := map[string]config.Provider{
		"openai": {
			Dialect: "openai",
			BaseURL: "https://api.openai.com/v1",
			Models:  []string{"gpt-4o"},
		},
	}
	r := route.New(providers)
	mux := createModelsMux(func() (*route.Router, error) {
		return r, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/models", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /openai/v1/models status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestModelsEndpoint_RouterBuildFailure(t *testing.T) {
	mux := createModelsMux(func() (*route.Router, error) {
		return nil, fmt.Errorf("no topology loaded")
	})

	req := httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}

	var env struct {
		Error map[string]interface{} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal error response failed: %v", err)
	}
	if env.Error == nil {
		t.Errorf("expected json body to contain \"error\" object, got body: %s", rec.Body.String())
	}
}
