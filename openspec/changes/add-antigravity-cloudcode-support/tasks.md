# Tasks

Reference: `proposal.md` (why), `design.md` (decisions D1–D6), `specs/antigravity-cloudcode/spec.md` (requirements). Tasks are ordered by dependency and sized to one session each. Follow `testing.md` (TDD, ≥80% coverage) and `coding-style.md` (contract/types/impl in separate files, `gofmt`, `%w` error wrapping).

## 1. Config & preset foundation

- [x] 1.1 Add an optional `Transport string` field (`json:"transport,omitempty"`) to `config.Provider`; parse it in topology loading. Providers without the field behave exactly as before.
- [x] 1.2 Teach `ValidateTopology` to accept `""` and `"cloudcode"` and to reject any other value with a clear error naming the provider and the invalid transport.
- [x] 1.3 Fix `AntigravityPreset` (`internal/preset/catalog.go`): set runtime base_url `https://daily-cloudcode-pa.googleapis.com`, bootstrap endpoint `https://cloudcode-pa.googleapis.com`, and `transport: "cloudcode"`; leave OAuth metadata (client id, scopes, endpoints) unchanged. Update `internal/preset/preset_test.go`.

## 2. Onboarding component (`internal/cloudcode`)

- [x] 2.1 Create `internal/cloudcode`: an `Onboarding` type exposing `ProjectID(ctx, accessToken) (string, error)`, backed by an in-memory cache keyed by access token with a ~1h TTL, concurrency-safe (collapse concurrent calls for the same token — mirror the credential refresh dedupe in `provider-credentials`).
- [x] 2.2 Implement `loadCodeAssist` (`POST cloudcode-pa.googleapis.com/v1internal:loadCodeAssist`, IDE headers + `Authorization: Bearer`), parse `cloudaicompanionProject`; fall back to `onboardUser` when no project is returned.
- [x] 2.3 Tests (`httptest`): cache hit/miss/expiry, `loadCodeAssist` success, `onboardUser` fallback, and error propagation. Errors never log tokens.

## 3. CloudCode executor

- [x] 3.1 Envelope builder: wrap a native Gemini payload into `{project, model, userAgent:"antigravity", requestType:"agent", requestId, request:{<payload>}}`; generate `requestId`.
- [x] 3.2 Endpoint + headers: non-stream `POST {base}/v1internal:generateContent`; stream `POST {base}/v1internal:streamGenerateContent?alt=sse`; set `Authorization: Bearer <token>` and `User-Agent: antigravity/ide/2.1.1 darwin/arm64`.
- [x] 3.3 Response relay: reuse the gemini dialect's response handling and usage scanner (confirm the response is unwrapped native Gemini — Open Question; stream + non-stream).
- [x] 3.4 Tests (`httptest`): envelope shape + `model` field, stream/non-stream endpoint selection, headers, response relay.

## 4. Proxy transport hook

- [x] 4.1 At the top of the per-hop loop in `internal/proxy/proxy.go`, branch on `provider.Transport == "cloudcode"` and delegate the send to the CloudCode executor; the default (empty) path is unchanged.
- [x] 4.2 Reuse the existing per-hop credential resolution to obtain the access token; resolve the onboarding project ID via `internal/cloudcode` and pass both into the executor.
- [x] 4.3 Tests: a `cloudcode` provider routes through the executor; a standard provider is byte-for-byte unchanged (regression); executor errors surface without crashing the hop loop.

## 5. Probe & config migration

- [x] 5.1 Confirm the dashboard model probe (`internal/probe/probe.go` `RunInProcess`) exercises the CloudCode path for an antigravity model (no code change expected); add a test asserting the probe drives the executor.
- [x] 5.2 Config migration: on topology load, set `transport: "cloudcode"` + CloudCode base_url for a provider named `antigravity` that lacks `transport`. Add a test.

## 6. End-to-end verification

- [x] 6.1 Resolve Open Questions against the live backend: confirm the model-ID form (verbatim vs `gemini-3.6-flash-tiered(medium)` mapping) and that generate responses are unwrapped native Gemini; adjust the mapping (kept in preset data, not code) if required.
- [x] 6.2 `gofmt -w .` ; `go build ./...` ; `go test ./...` green, with ≥80% coverage on `internal/cloudcode` and the executor.
- [x] 6.3 Manual: `go run . serve`, dashboard **Test** on an antigravity model (`gemini-3.6-flash-medium`) → expect success or a real upstream status, not a generic-endpoint 404.
