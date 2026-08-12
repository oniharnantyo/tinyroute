## Context

Per-request observability today is async-only: `history.Recorder`
(`internal/history/recorder.go`) appends a JSONL line off the critical path from
`proxy.recordOutcome` (`internal/proxy/proxy.go:221`), *after* the client is
answered. And `requestHandler` (`internal/cli/serve.go:287`) returns early at six
points (body `400`, parse `400`, key `401`, rate `429`, routing `500`, resolve
`404`) before the proxy runs, so those requests reach neither the proxy nor the
recorder. There is no synchronous line-per-request emission; the only `log.Printf`
sites are lifecycle messages and a few warnings.

This change adds a structured `log/slog` access line per request, emitted at
completion. Constraints inherited from the codebase and `proposal.md`:

- Standard library only; the proxy's D14 "core + stdlib only" import boundary
  must hold (`internal/proxy/proxy.go:1`).
- Surgical, single-concern edits; new concern → own package (`coding-style.md`).
- The `history` store remains the sole owner of upstream-outcome detail
  (`proposal.md` Non-goal); the access log correlates to it by `request_id`, not
  by duplication.

One subtlety surfaced while designing: the proposal's "What Changes" listed
`key_id` and `model` as access-line attributes. Those values are computed inside
`requestHandler` (downstream of any outer middleware) and cannot be observed from
a transport-layer wrapper without a context-writeback bag. Decision D4 below
revises that bullet — see Open Questions.

## Goals / Non-Goals

**Goals:**

- Exactly one `slog` record per HTTP request, at completion, covering *every*
  outcome including the six pre-proxy early returns.
- Structured transport + correlation fields: `method`, `path`, `status`,
  `bytes`, `latency`, `remote`, `request_id`.
- Configurable format (`text`/`json`) and level (`debug`/`info`/`warn`/`error`).
- D14 intact: the `internal/proxy` package imports no new concern.

**Non-Goals:** (mirror the proposal) no duplication of upstream fields
(provider/attempts/outcome/tokens) — those stay in `history`; no body capture in
the access line; no sampling; no `slog.SetDefault` bridging; no log shipping.

## Decisions

### D1. Placement: an outer middleware wrapping the mux

The logger is a `func(http.Handler) http.Handler` applied as
`accesslog.Middleware(logger)(mux)` in `serve.go`, so it observes the request
*before* `requestHandler` and the response *after*. This is the only placement
that sees the pre-proxy early returns — logging inside the proxy would silently
drop every `401`/`429`/`404`/parse failure, which is precisely the traffic an
access log exists to catch.

*Alternatives rejected:* log inside `proxy.Handler` (blind to pre-proxy failures);
log inside `requestHandler` (misses `/v1/models` and any future non-proxied
route).

### D2. `log/slog`, injected as `*slog.Logger` in `proxy.Deps`

`log/slog` is standard library (Go 1.21+; module is `go 1.26.4`), so a
`*slog.Logger` is a legal `proxy.Deps` field under D14 without introducing a
`core.Logger` abstraction. One root logger is built in `serve.go` from config;
the proxy receives it via `Deps` and uses it only to re-level its existing
`log.Printf` sites (D7). The middleware receives the same logger directly.

*Alternatives rejected:* zap/zerolog (third-party, would require a `core.Logger`
interface to respect D14 and would shed structure at the boundary); plain
`log.Printf` (loses structured filtering of status/latency/bytes).

### D3. A `ResponseWriter` interceptor captures status and bytes

The stdlib `http.ResponseWriter` exposes neither the status code nor bytes
written after the fact. The middleware wraps `w` in a small struct that records
the argument of `WriteHeader` and counts bytes through `Write`, then reads both
at completion. Standard, well-understood pattern; no third-party dependency.

### D4. The line is transport + `request_id` only — `key_id`/`model` are NOT propagated

The access line carries `method`, `path`, `status`, `bytes`, `latency`, `remote`,
and `request_id`. It does **not** carry `key_id` or `model`. *(This revises the
proposal's "What Changes" bullet that listed them.)*

`key_id` and `model` are known only inside `requestHandler`, downstream of the
middleware. Including them would require a per-request attribute bag written
back through context — mechanism that buys little, because `request_id`
correlation already retrieves `key_id`, `model`, *and* the full upstream outcome
from `history` on demand. Dropping them keeps the implementation to a middleware
plus an interceptor with no context-writeback, and honors the thin-outer non-goal
consistently.

*Alternative considered and deferred:* an `accesslog.Attrs` context bag that
`requestHandler` (in `internal/cli`, outside D14) populates with `key_id`/`model`.
Clean w.r.t. D14, but over-engineering for the MVP; revisit if live per-key
grouping proves necessary (then `request_id` joins no longer suffice).

### D5. `request_id` is owned by the middleware and shared with history

For access-log ↔ history correlation to hold, both must use the same id. The
middleware generates (or honors `X-Request-Id`) the `request_id` at entry and
stores it in the request context; `requestHandler` reads it from context for
`RequestCtx.RequestID` instead of calling the local `requestID(r)` helper
(`serve.go:362`). `request_id` is a transport-layer correlation id, so the
access layer owning it is architecturally correct, and `history.rec.ID`
(`proxy.go:233`) then matches the access line by construction.

### D6. Config: two env vars, validated fail-fast in `LoadService`

`TINYROUTE_LOG_FORMAT` (`text` | `json`, default `text`) and `TINYROUTE_LOG_LEVEL`
(`debug` | `info` | `warn` | `error`, default `info`), added to the `known` map at
`service.go:79`. Invalid values fail in `LoadService` exactly as malformed
durations and half-set TLS already do (`service.go:56`–`76`), rather than
silently degrading. The root `*slog.Handler` (`JSONHandler` or `TextHandler`) is
constructed once from these.

### D7. Existing `log.Printf` sites are re-leveled; lifecycle logs are untouched

`proxy.go:279` (auth-error cooldown) and `proxy.go:321` (mid-stream failure)
become `logger.Warn`; invariant violations become `logger.Error`; per-hop attempt
detail becomes `logger.Debug`. Server lifecycle messages in `serve.go`
("listening on…", "shutting down…") stay on the stdlib `log` package — a
different audience, and `slog.SetDefault` bridging is a non-goal.

### D8. SSE latency needs no special handling

For streaming responses the handler blocks until the stream drains, so
`time.Since(start)` measured at middleware return captures the full stream
duration. The interceptor's `WriteHeader` records the committed status (always
`200` for a committed SSE relay) before the body flows.

## Risks / Trade-offs

- **[Proposal bullet revised — `key_id`/`model` dropped from the line]** →
  Retrievable via `request_id` join to `history`. *Mitigation:* sync the proposal
  bullet before archiving (see Open Questions).
- **[Log volume; `slog` has no sampling]** → A runaway client could flood INFO.
  *Mitigation:* the per-key `RateLimiter` (`internal/auth`) already bounds blast
  radius; a sampling `slog.Handler` is a deferred follow-on.
- **[`slog` writes to stderr by default]** → Conventional for Go, but some log
  shippers prefer stdout. *Mitigation:* acceptable for now; a destination option
  is deferred (Open Questions).
- **[`request_id` coupling between `accesslog` and `internal/cli`]** → Minimal,
  via one documented context key. *Mitigation:* the key lives in `accesslog`;
  `cli` is a consumer, consistent with how `cli` already imports sibling packages.
- **[Re-leveling existing `log.Printf` changes observable output]** → Warnings
  that were plain text become structured records. *Mitigation:* same information,
  better shape; no caller is known to parse them.

## Migration Plan

Single release, no data migration. Two env vars are additive with safe defaults
(`text`/`info`), so existing deployments behave as before plus one structured
line per request to stderr. No on-disk state changes; the `history` JSONL file
and its schema are untouched. Rollback is `git revert`.

## Open Questions

- Sync the proposal's `key_id`/`model` bullet with D4? **Recommended: yes** —
  update `proposal.md` so proposal and design agree before `tasks.md`.
- Destination: stderr (slog default) vs stdout for container log shippers? Lean
  stderr for now; defer a `TINYROUTE_LOG_DEST` option.
- Implement the `DEBUG` per-hop line now, or defer to a follow-on? Lean defer —
  ship the INFO access line first.