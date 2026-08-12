package gemini_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/credential"
	"github.com/oniharnantyo/tinyroute/internal/dialect"
	"github.com/oniharnantyo/tinyroute/internal/dialect/gemini"
)

func TestGeminiDialectRegistration(t *testing.T) {
	d, ok := dialect.ByName("gemini")
	if !ok {
		t.Fatalf("gemini dialect not registered")
	}
	if d.Name() != "gemini" {
		t.Errorf("d.Name() = %q, want \"gemini\"", d.Name())
	}
	paths := d.Paths()
	if len(paths) == 0 || paths[0] != "/v1beta/models" {
		t.Errorf("d.Paths() = %v, want [\"/v1beta/models\"]", paths)
	}
}

func TestAuthHeaders(t *testing.T) {
	d := &gemini.Dialect{}

	t.Run("Static API key", func(t *testing.T) {
		cred := credential.TokenResult{
			Value: "my-api-key",
			Kind:  credential.KindStatic,
		}
		headers := d.AuthHeaders(cred, nil)
		if got := headers.Get("x-goog-api-key"); got != "my-api-key" {
			t.Errorf("x-goog-api-key = %q, want %q", got, "my-api-key")
		}
		if got := headers.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want empty", got)
		}
		if got := headers.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
	})

	t.Run("OAuth Bearer Token", func(t *testing.T) {
		cred := credential.TokenResult{
			Value: "my-oauth-token",
			Kind:  credential.KindOAuthBearer,
		}
		headers := d.AuthHeaders(cred, nil)
		if got := headers.Get("Authorization"); got != "Bearer my-oauth-token" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer my-oauth-token")
		}
		if got := headers.Get("x-goog-api-key"); got != "" {
			t.Errorf("x-goog-api-key = %q, want empty", got)
		}
	})

	t.Run("Header Overrides", func(t *testing.T) {
		cred := credential.TokenResult{Value: "key", Kind: credential.KindStatic}
		override := "custom-value"
		var remove *string = nil
		headers := d.AuthHeaders(cred, map[string]*string{
			"X-Custom-Header": &override,
			"Content-Type":    remove,
		})
		if got := headers.Get("X-Custom-Header"); got != "custom-value" {
			t.Errorf("X-Custom-Header = %q, want custom-value", got)
		}
		if got := headers.Get("Content-Type"); got != "" {
			t.Errorf("Content-Type = %q, want empty", got)
		}
	})
}

func TestUsageScanner(t *testing.T) {
	d := &gemini.Dialect{}
	scanner := d.NewUsageScanner()

	if scanner.Usage() != nil {
		t.Fatalf("expected nil initial usage")
	}

	chunk1 := []byte(`{"candidates":[],"usageMetadata":{"promptTokenCount":15,"candidatesTokenCount":5,"totalTokenCount":20}}`)
	scanner.Observe(chunk1)

	u1 := scanner.Usage()
	if u1 == nil {
		t.Fatalf("expected non-nil usage after chunk 1")
	}
	if u1.InputTokens != 15 || u1.OutputTokens != 5 {
		t.Errorf("u1 = %+v, want InputTokens: 15, OutputTokens: 5", u1)
	}

	chunk2 := []byte(`{"candidates":[],"usageMetadata":{"promptTokenCount":15,"candidatesTokenCount":25,"totalTokenCount":40,"cachedContentTokenCount":10}}`)
	scanner.Observe(chunk2)

	u2 := scanner.Usage()
	if u2 == nil {
		t.Fatalf("expected non-nil usage after chunk 2")
	}
	if u2.InputTokens != 15 || u2.OutputTokens != 25 || u2.CacheReadTokens != 10 {
		t.Errorf("u2 = %+v, want InputTokens: 15, OutputTokens: 25, CacheReadTokens: 10", u2)
	}

	// Ignore malformed chunk
	scanner.Observe([]byte(`not json`))
	if !reflect.DeepEqual(scanner.Usage(), u2) {
		t.Errorf("usage changed after malformed chunk: %+v", scanner.Usage())
	}
}

func TestParseRequestAndRewriteModel(t *testing.T) {
	d := &gemini.Dialect{}

	body := []byte(`{
		"model": "gemini-1.5-pro",
		"stream": true,
		"systemInstruction": {"parts":[{"text":"System prompt"}]},
		"contents": [{"role":"user","parts":[{"text":"User question"}]}]
	}`)

	pr, err := d.ParseRequest(body)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}
	if pr.Model != "gemini-1.5-pro" {
		t.Errorf("pr.Model = %q, want gemini-1.5-pro", pr.Model)
	}
	if !pr.Stream {
		t.Errorf("pr.Stream = false, want true")
	}
	if len(pr.SessionInputs) != 2 {
		t.Errorf("len(pr.SessionInputs) = %d, want 2", len(pr.SessionInputs))
	}

	rewritten, err := d.RewriteModel(body, "gemini-1.5-flash")
	if err != nil {
		t.Fatalf("RewriteModel failed: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(rewritten, &raw); err != nil {
		t.Fatalf("unmarshal rewritten failed: %v", err)
	}
	if raw["model"] != "gemini-1.5-flash" {
		t.Errorf("rewritten model = %v, want gemini-1.5-flash", raw["model"])
	}
}

func TestWriteError(t *testing.T) {
	d := &gemini.Dialect{}
	rec := httptest.NewRecorder()

	d.WriteError(rec, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid model name")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", rec.Header().Get("Content-Type"))
	}

	var errResp struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("unmarshal error response failed: %v", err)
	}
	if errResp.Error.Code != 400 || errResp.Error.Message != "Invalid model name" || errResp.Error.Status != "INVALID_ARGUMENT" {
		t.Errorf("errResp = %+v", errResp)
	}
}
