## Why

tinyroute has no real-time per-request visibility. The only per-request artifact
today is `history.Recorder` (`internal/history/recorder.go`), which appends a
JSONL line to `requests.jsonl` *off the critical path* — fired from a goroutine
in `proxy.recordOutcome` (`internal/proxy/proxy.go:221`) *after* the client has
already been answered. It is a replay/audit store, not a live observability
stream: an operator watching the process cannot tell whether traffic is flowing,
who is erroring, or how fast requests complete.

Worse, a large class of requests is invisible to even that record.
`requestHandler` (`internal/cli/serve.go:287`) returns early at six points —
malformed body (`400`), unparseable body (`400`), bad key (`401`), rate-limited
(`429`), routing unavailable (`500`), and unresolved model (`404`) — *before*
the proxy's attempt loop ever runs. None of those reach `recordOutcome`, so a
misconfigured client hammering the gateway with bad keys surfaces as silence.
The only `log.Printf` call sites today are server lifecycle messages and a
handful of warnings (auth-error cooldown `proxy.go:279`, mid-stream failure
`proxy.go:321`); nothing emits a line per request.

An LLM gateway's operationally relevant fields — status, latency, key id, model
— are naturally numeric/structured, which is exactly what `log/slog` (standard
library since Go 1.21; the module is `go 1.26.4`) is built for. Plain
`log.Printf` would force them into a string template and lose filterability.

## What Changes

- **One structured access line per request at completion (INFO).** A new
  `internal/accesslog/` package exposes a middleware that wraps the mux in
  `serve.go`, installs a status/byte-capturing `http.ResponseWriter`
  interceptor, times the request, and emits one `slog` record at completion with
  `method`, `path`, `status`, `bytes`, `latency`, `remote`, and `request_id`. The
  status/bytes interceptor is required because the stdlib `ResponseWriter` exposes
  neither after `WriteHeader`; without it a log line could say a request happened
  but not what status it got.
- **`log/slog` is the logger; the proxy's D14 boundary stays intact.** Because
  `log/slog` is standard library, a `*slog.Logger` can sit directly in
  `proxy.Deps` as a field without introducing a `core.Logger` abstraction — it is
  a stdlib type, not a sibling internal package, so the proxy's
  "core-and-stdlib-only" import rule holds.
- **Two new env vars** following the existing `TINYROUTE_` pattern
  (`internal/config/service.go`): `TINYROUTE_LOG_FORMAT` (`text` | `json`,
  default `text`) and `TINYROUTE_LOG_LEVEL` (`debug` | `info` | `warn` |
  `error`, default `info`), both added to the `known` map at `service.go:79` so
  unknown-setting warnings stay accurate. The root `*slog.Logger` is built once
  in `serve.go` from these.
- **Existing ad-hoc `log.Printf` sites are re-leveled onto slog:** per-hop
  attempt detail becomes `DEBUG` (failover forensics, cranked on during
  incidents), provider cooldowns and mid-stream failures become `WARN`, and
  invariant violations (missing `RequestCtx`, marshal failure) become `ERROR`.
- **Server lifecycle messages** (startup "listening on…", shutdown) stay on the
  stdlib `log` package — a different audience from per-request traffic.

### Non-goals (recorded so they are not relitigated)

- **The access line does NOT duplicate history's upstream fields.** Provider,
  per-attempt status/latency, outcome, and token usage remain owned solely by
  `history.Recorder` as the canonical replay-grade record. The access log and
  the history correlate by the shared `request_id` (already produced at
  `serve.go:333`), not by copy. This is the "thin outer" decision: one concern,
  one owner, no drift.
- **No sampling.** `slog` has no built-in sampling. The per-key `RateLimiter`
  (`internal/auth`) already bounds blast radius; a high-QPS sampling handler is a
  deferred follow-on if ever needed.
- **No `slog.SetDefault` bridging of stdlib `log`.** That is a repo-wide
  logging-policy decision and is out of scope here.
- **No request/response body logging in the access line.** Body capture is
  already owned by history's `TINYROUTE_CAPTURE=full` mode (with its existing
  PII warning at `recorder.go:25`); the access log carries metadata only.
- **No log shipping, aggregation, or external sink.** Output goes to stdout via
  the chosen `slog` handler; integration with Loki/Datadog is the operator's job.

## Capabilities

### New Capabilities

- `access-logging`: tinyroute SHALL emit one structured `slog` record per HTTP
  request at completion — covering *all* outcomes including the pre-proxy early
  returns (auth, rate-limit, routing, parse failures) — with transport and
  gateway-context attributes, configurable format and level, while leaving the
  replay-grade `history` store as the sole owner of upstream-outcome detail.

## Impact

- **Code**: new `internal/accesslog/` package (middleware + `ResponseWriter`
  interceptor); `internal/config/service.go` (two env vars + `known`-map
  entries); `internal/cli/serve.go` (build root logger, wrap mux, inject logger
  into `Deps`); `internal/proxy/proxy.go` (add `Logger *slog.Logger` to `Deps`;
  re-level existing `log.Printf` sites; optional `DEBUG` per-hop line). The
  middleware wraps the whole mux, so it observes the early-return paths the
  proxy never sees.
- **Behavior change**: stdout, previously silent per request, now emits one
  structured line per request. Two new env vars are recognized. No on-disk state
  changes; the `history` JSONL file and its schema are untouched.
- **Tests**: `internal/accesslog` tests asserting the interceptor captures
  status, bytes, and latency; that a pre-proxy `401`/`429`/`404` still produces a
  line; and that `request_id`/`key_id`/`model` are pulled from
  `proxy.RequestCtx`. A `config` test for the two new env vars and their
  defaults.