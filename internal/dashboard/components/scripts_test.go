package components

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScriptsBundleIncludesComponentJS(t *testing.T) {
	// Force the embedded production bundle so the test does not depend on
	// the process working directory (dev mode walks a relative path).
	t.Setenv("GO_ENV", "production")

	js, _, _ := bundle()
	if !strings.Contains(string(js), `data-slot="input-group-addon"`) {
		t.Error("bundle missing inputgroup.js content")
	}
	if !strings.HasPrefix(scriptsSrc(), "/components/shadcn-templ-") {
		t.Errorf("unexpected bundle src: %s", scriptsSrc())
	}
}

func TestScriptsHandlerServesBundle(t *testing.T) {
	t.Setenv("GO_ENV", "production")

	rec := httptest.NewRecorder()
	ScriptsHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/components/shadcn-templ.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/javascript" {
		t.Errorf("unexpected content type: %s", ct)
	}
	if !strings.Contains(rec.Body.String(), `data-slot="input-group-addon"`) {
		t.Error("served bundle missing inputgroup.js content")
	}
}

func TestScriptsHandlerRejectsUnknownBundleName(t *testing.T) {
	t.Setenv("GO_ENV", "production")

	rec := httptest.NewRecorder()
	ScriptsHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/components/other.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown bundle name, got %d", rec.Code)
	}
}
