TDD-ordered: write the invariant test first (RED), make it pass (GREEN), then
verify. One new dependency (`modernc.org/sqlite`, pure Go); surgical edits to
`core`, `history`, `proxy`, `config`, and `cli`. D14 holds: the proxy never
imports the SQLite package.

## 1. Data model and contracts (`internal/core`)

- [x] 1.1 Add `CacheCreationTokens int64` to `core.Usage` (RED: test it
      marshals/unmarshals and is distinct from `CacheReadTokens`).
- [x] 1.2 Add `Latency time.Duration` and serving `Provider string` to
      `core.RequestRecord` (RED: test they round-trip through JSON).
- [x] 1.3 Remove the stale `// Second implementation: sqlite.` comment from
      `core.Recorder` (`internal/core/interfaces.go`).
- [x] 1.4 `gofmt -w .` and `go test ./internal/core`.

## 2. Dialect usage scanners surface cache-creation (`internal/dialect/*`)

- [x] 2.1 Extend the anthropic usage scanner to extract
      `cache_creation_input_tokens` into `Usage.CacheCreationTokens` (RED).
- [x] 2.2 Audit the openai usage scanner: confirm
      `prompt_tokens_details.cached_tokens` maps to `CacheReadTokens`; document
      whether OpenAI reports a cache-creation equivalent (if not, leave
      `CacheCreationTokens` zero for that dialect).
- [x] 2.3 Tests for each scanner asserting the cache fields are populated from a
      realistic SSE usage chunk.

## 3. SQLite store — lifecycle and schema (`internal/history/sqlite/db.go`)

- [x] 3.1 `go get modernc.org/sqlite`; confirm `go build ./...` still produces a
      single CGo-free binary.
- [x] 3.2 `Open(path)` creates the DB, sets PRAGMAs (`journal_mode=WAL`,
      `busy_timeout=5000`, `synchronous=NORMAL`, `foreign_keys=ON`).
- [x] 3.3 `Migrate` applies the `CREATE TABLE requests (...)` + three indexes
      (`idx_requests_ts`, `idx_requests_provider`, `idx_requests_session`) with
      `IF NOT EXISTS`, tracked by `PRAGMA user_version`. Idempotent on re-`Open`.
- [x] 3.4 `Close` releases the handle. Tests: Open creates the schema; re-Open is
      idempotent; `user_version` advances and is stable.

## 4. SQLite store — write path (`internal/history/sqlite/store.go`)

- [x] 4.1 Implement `core.Recorder`: `INSERT` one row from `core.RequestRecord`,
      mapping `Usage`→token columns, `Latency`→`latency_ms` (ms), serving
      `Provider`, blob refs, and `attempts` marshaled to JSON text.
- [x] 4.2 Decide duplicate-id policy (`INSERT OR IGNORE` vs replace) and test it;
      an id collision must not corrupt the row.
- [x] 4.3 Test: `Record` persists all fields; `provider` is empty for a failed
      outcome; nil `Usage` defaults all token columns to zero; `attempts`
      round-trips through JSON.

## 5. Query path (`internal/history/query.go` + `internal/history/sqlite/query.go`)

- [x] 5.1 Define the history-package-local `Querier` interface and a `Summary`
      type (thin row minus blob refs) in `internal/history/query.go`.
- [x] 5.2 Implement `List(ctx, filter{Provider, From, To, Limit, Cursor})`
      returning most-recent-first results with an opaque cursor derived from
      `(ts, id)` of the last row.
- [x] 5.3 RED→GREEN tests: filter by provider only; by date range only; combined;
      pagination yields the full set with no dupes and terminates with an empty
      cursor; an empty `Limit`/`Cursor` is handled; no-match returns empty, no
      error.
- [x] 5.4 Confirm `List` returns refs only — full payloads are fetched on demand
      via the existing `BlobStore.Get`, not by `List`.

## 6. Restructure `internal/history` and retire JSONL

- [x] 6.1 Delete `internal/history/recorder.go` (the JSONL `Recorder` and its
      `sync.Mutex`); confirm no remaining caller imports it.
- [x] 6.2 Leave `internal/history/blobstore.go` and `session.go` at the package
      root (shared by `sqlite/` and any future engine).
- [x] 6.3 Verify the package layout is now: `history/{blobstore.go, session.go,
      query.go, sqlite/}`.
- [x] 6.4 Confirm `internal/proxy` still compiles against the unchanged
      `core.Recorder` — no new import (D14).

## 7. Proxy capture enrichment (`internal/proxy/proxy.go`)

- [x] 7.1 Stamp `start := time.Now()` at the top of `Handler`; pass
      `time.Since(start)` into `recordOutcome` → `RequestRecord.Latency` (RED:
      recorded row has a positive `latency_ms`).
- [x] 7.2 Derive the serving provider from `attempts` (last `2xx` hop) and set
      `RequestRecord.Provider` (RED: populated on success; empty on
      `chain_exhausted` / `no_route` / `auth_failed`).
- [x] 7.3 Add `XlatedReqBlob` / `RawRespBlob` to `core.RequestRecord`; clarify
      existing fields as `ReqBlob` (raw client request) and `ClientRespBlob`
      (client-facing response).
- [x] 7.4 In `recordOutcome`, capture the translated provider request and the raw
      upstream response for the **winning hop only** into `BlobStore` (RED: full
      capture populates all four refs; disabled leaves them empty; retried/failed
      hops are not blobbed).
- [x] 7.5 Keep capture gated behind `TINYROUTE_CAPTURE=full`; retain the PII
      warning.

## 8. Wiring and config (`internal/config`, `internal/cli/serve.go`)

- [x] 8.1 Add `TINYROUTE_HISTORY_DB` to the `known` map in
      `internal/config/service.go`; resolve the default location (Open Question:
      alongside the existing history path) and validate fail-fast like the other
      settings.
- [x] 8.2 In `serve.go`: `Open` the SQLite store at startup, run `Migrate`,
      inject it as `core.Recorder` into `proxy.Deps`, expose `Querier` to the CLI,
      and `Close` on shutdown. (Disjoint region from the in-flight
      `models-endpoint-conformance` `/v1/models` edits.)
- [x] 8.3 Tests: config validates the new var/defaults; `serve` constructs the
      store and closes it on shutdown.

## 9. Verify and accept

- [x] 9.1 `go test ./...` — all green; coverage ≥80% on touched packages
      (`internal/core`, `internal/history`, `internal/history/sqlite`,
      `internal/proxy`, `internal/config`, dialect scanners).
- [x] 9.2 `gofmt -w .` and `go vet ./...` clean.
- [x] 9.3 Manual smoke: `go run . serve`, send a request, then read the DB (or
      via a temporary query helper) — confirm the row exists with latency,
      serving provider, usage (incl. cache-creation when the upstream reports
      it), and the four blob refs under full capture.
- [x] 9.4 Cross-check: every scenario in `specs/session-history/spec.md` maps to
      a passing test.
- [x] 9.5 `openspec validate --changes session-history-sqlite` clean.

## 10. Follow-ons (explicitly NOT in this change)

- [ ] 10.1 One-shot JSONL → SQLite importer (deferred; non-goal here).
- [ ] 10.2 Retention/rotation policy for the SQLite DB (deferred; Open Question).
- [ ] 10.3 `tinyroute history list` CLI command built on `Querier`
      (interactive-first per `.claude/rules/cli-interactivity.md`).
