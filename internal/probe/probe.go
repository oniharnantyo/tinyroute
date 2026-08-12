package probe

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/credential"
	"github.com/oniharnantyo/tinyroute/internal/dialect"
	"github.com/oniharnantyo/tinyroute/internal/proxy"
)

// Result holds the status code and latency of a probe test.
type Result struct {
	StatusCode int
	Latency    time.Duration
	Err        error
}

// ProbeBodyFor returns a minimal test body for the given dialect.
func ProbeBodyFor(dialectName string) string {
	switch dialectName {
	case "anthropic":
		// max_tokens 16, not 1: some upstreams emit no content token before a
		// 1-token budget is spent, so a 1-token probe yields a false negative.
		return `{"model":"probe","max_tokens":16,"messages":[{"role":"user","content":"ping"}]}`
	case "gemini":
		// Gemini generateContent uses "contents"/"parts", not the OpenAI
		// "messages" shape — the CloudCode backend rejects "messages" with
		// INVALID_ARGUMENT "Unknown name \"messages\"". RewriteModel adds the
		// model field; the cloudcode executor strips it into the envelope.
		return `{"contents":[{"role":"user","parts":[{"text":"ping"}]}]}`
	default:
		return `{"model":"probe","messages":[{"role":"user","content":"ping"}]}`
	}
}

// RequestBody builds a minimal probe request body for dialectName with the target
// model substituted in via the dialect's RewriteModel. Shared by the direct probe
// (TestModel) and the in-process gateway probe (RunInProcess).
func RequestBody(dialectName, model string) ([]byte, error) {
	d, ok := dialect.ByName(dialectName)
	if !ok {
		return nil, fmt.Errorf("unknown dialect %q", dialectName)
	}
	return d.RewriteModel([]byte(ProbeBodyFor(dialectName)), model)
}

// TestModel executes a probe call to the provider endpoint for the given model.
func TestModel(ctx context.Context, provName string, prov config.Provider, targetModel string, credStorePath string, timeout time.Duration) (int, time.Duration, error) {
	d, ok := dialect.ByName(prov.Dialect)
	if !ok {
		return 0, 0, fmt.Errorf("unknown dialect %q for provider %q", prov.Dialect, provName)
	}

	probeBody, err := d.RewriteModel([]byte(ProbeBodyFor(prov.Dialect)), targetModel)
	if err != nil {
		return 0, 0, fmt.Errorf("build probe request: %w", err)
	}

	outboundPaths := d.Paths()
	if len(outboundPaths) == 0 {
		return 0, 0, fmt.Errorf("dialect %q declares no outbound path", prov.Dialect)
	}
	url := strings.TrimRight(prov.BaseURL, "/") + outboundPaths[0]

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(probeBody))
	if err != nil {
		return 0, 0, fmt.Errorf("create probe request: %w", err)
	}

	credStore, _ := credential.NewStore(credStorePath)
	tokRes, _ := prov.BuildCredential(provName, credStore).Token(ctx)
	req.Header = d.AuthHeaders(tokRes, prov.Headers)

	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return 0, elapsed, fmt.Errorf("unreachable (%v) in %s", err, elapsed.Round(time.Millisecond))
	}
	defer resp.Body.Close()

	return resp.StatusCode, elapsed, nil
}

// RunInProcess probes a model by executing it through the gateway's real request
// path in-process — no network boundary and no API-key auth. It builds the proxy
// request context the same way requestHandler does (but with a synthetic keyID),
// resolves the route via resolve (typically router.Resolve), and drives the proxy
// attempt loop (handler) against an httptest recorder. The returned statusCode is
// exactly what a real client would receive: 200 on success, the upstream status on
// a committed failure, 502 when all hops are exhausted.
//
// This mirrors OmniRoute's modelTestRunner pattern (in-process handler invocation
// with a synthetic request) and avoids the auth bypass an HTTP loopback would need.
func RunInProcess(ctx context.Context, provName, dialectName, model string, resolve func(name, model string) (core.ResolvedRoute, error), handler http.HandlerFunc, timeout time.Duration) (int, time.Duration, error) {
	d, ok := dialect.ByName(dialectName)
	if !ok {
		return 0, 0, fmt.Errorf("unknown dialect %q", dialectName)
	}

	body, err := RequestBody(dialectName, model)
	if err != nil {
		return 0, 0, fmt.Errorf("build probe request: %w", err)
	}

	parsed, err := d.ParseRequest(body)
	if err != nil {
		return 0, 0, fmt.Errorf("parse probe request: %w", err)
	}

	// Resolve via the prefixed provider:model form so a known provider+model
	// always routes directly, independent of explicit route configuration. The
	// bare model is still used for the request body built above (RequestBody).
	resolved, err := resolve(dialectName, provName+":"+model)
	if err != nil {
		return 0, 0, fmt.Errorf("resolve route for %s:%s: %w", provName, model, err)
	}

	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	rc := &proxy.RequestCtx{
		Dialect:   d,
		Route:     resolved,
		Parsed:    parsed,
		RequestID: "internal-probe",
		KeyID:     "internal-probe",
	}

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	probeCtx, cancel := context.WithTimeout(proxy.WithRequestContext(req.Context(), rc), timeout)
	defer cancel()
	req = req.WithContext(probeCtx)

	rec := httptest.NewRecorder()
	start := time.Now()
	handler(rec, req)
	elapsed := time.Since(start)

	// Guard against a false-positive OK: a 2xx with an empty body means the
	// provider accepted the request but produced nothing — a known failure mode
	// on tiny probes for some upstreams. Treat it as a probe failure rather than
	// reporting success. Conservative: only flags a blatantly empty body.
	if rec.Code >= 200 && rec.Code < 300 && len(bytes.TrimSpace(rec.Body.Bytes())) == 0 {
		return rec.Code, elapsed, fmt.Errorf("provider returned an empty response body (HTTP %d)", rec.Code)
	}

	return rec.Code, elapsed, nil
}
