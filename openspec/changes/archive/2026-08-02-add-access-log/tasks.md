# add-access-log — Tasks

Implementation breakdown. Reference: `proposal.md` (what/why),
`design.md` (decisions D1–D8), `specs/access-logging/spec.md` (normative
requirements + scenarios). Each task is verifiable.

## 1. Config: log format and level

- [x] 1.1 Add `LogFormat` and `LogLevel` fields to `config.Service` (`internal/config/service.go`), read from `TINYROUTE_LOG_FORMAT` (default `text`) and `TINYROUTE_LOG_LEVEL` (default `info`).
- [x] 1.2 Validate both in `LoadService`: reject any `LogFormat` outside `text|json` and any `LogLevel` outside `debug|info|warn|error`, failing fast with a named error (mirror the existing duration/TLS validation at `service.go:56`–`76`); add both keys to the `known` map (`service.go:79`).
- [x] 1.3 Add `internal/config` tests covering: defaults when unset, each accepted value, and rejection of each invalid value.

## 2. accesslog package (TDD)

- [x] 2.1 Create `internal/accesslog/` with a status/byte-capturing `ResponseWriter` interceptor that records the argument of `WriteHeader` and counts bytes through `Write`. Test: a `429` status and an `N`-byte body are both captured (spec: *Captured status and bytes reflect the actual response*).
- [x] 2.2 Add a request-id context helper (`WithRequestID`/`RequestID`) that honors a caller-supplied id when present and generates one otherwise. Test: header honored; id generated when absent (spec: *caller-supplied request id is honored*).
- [x] 2.3 Implement `Middleware(logger *slog.Logger) func(http.Handler) http.Handler` that installs the interceptor, sets the request id in context, times the request, and emits exactly one `INFO` record at completion with `method`, `path`, `status`, `bytes`, `latency`, `remote`, `request_id`. Test: exactly one record emitted, all fields populated (spec: *transport and correlation fields*).
- [x] 2.4 Test that a handler returning a non-`2xx` early (e.g. `401`) still produces exactly one record — proving pre-proxy early returns are observed (spec: *pre-proxy authentication failure is logged*).

## 3. serve.go wiring

- [x] 3.1 Build the root `*slog.Logger` once in `cmdServe` from `svc.LogFormat`/`svc.LogLevel` (`JSONHandler` when `json`, else `TextHandler`).
- [x] 3.2 Wrap the mux with `accesslog.Middleware(logger)` in `cmdServe` so every route — proxied chat paths and `GET /v1/models` — is observed (D1).
- [x] 3.3 Change `requestHandler` to read `request_id` from the accesslog context instead of calling the local `requestID(r)` helper (`serve.go:362`), so `RequestCtx.RequestID` matches the access record; remove the now-unused `requestID` helper (D5, spec: *correlate to request history by request identifier*).
- [x] 3.4 Confirm `GET /v1/models` produces one access record (spec: *non-proxied route is logged*).

## 4. Proxy: inject logger, re-level existing sites

- [x] 4.1 Add `Logger *slog.Logger` to `proxy.Deps` (`internal/proxy/proxy.go`) — a stdlib type, so D14 holds (D2).
- [x] 4.2 Replace `log.Printf` at the auth-error cooldown (`proxy.go:279`) and mid-stream failure (`proxy.go:321`) with `deps.Logger.Warn`; route the "missing request context" invariant (`proxy.go:75`) to `deps.Logger.Error` (D7).
- [x] 4.3 Pass the root logger from `cmdServe` into `proxy.Deps.Logger`.

## 5. Proposal sync and verification

- [x] 5.1 Sync `proposal.md`: drop `key_id` and `model` from the access-line attribute list in "What Changes" so proposal and design D4 agree (design Open Question, recommended: yes).
- [x] 5.2 `gofmt -w .` and `go build ./...` clean.
- [x] 5.3 `go test ./...` green; confirm `internal/accesslog` and `internal/config` coverage ≥ 80% (`testing.md`).
- [x] 5.4 Manual smoke: `go run . serve`, send one request with a bad key and one valid; confirm one structured line per request on stderr and that the access `request_id` matches the corresponding entry in the history JSONL.
- [x] 5.5 `openspec validate add-access-log` passes and `openspec status --change add-access-log` shows all artifacts done.