## Why

tinyroute records every proxied request through `history.Recorder`
(`internal/history/recorder.go`), which appends a JSONL line to `requests.jsonl`
off the critical path from `proxy.recordOutcome` (`internal/proxy/proxy.go:264`).
JSONL is an excellent append-only audit log but a poor query target: pagination,
date-range, and per-provider filtering each require an O(n) full-file scan —
worse across the `.1` rotation sibling — with no index to seek on. The intended
near-term usage surface — *"list requests, paginated, filtered by provider and a
start/end date"* — is exactly the workload JSONL cannot serve at scale.

Two further gaps make "now" the right moment. First, the recorded data is
incomplete relative to what operators need: token usage lacks cache-creation
(cache-write) counts; there is no end-to-end latency field (only per-hop
`elapsed`); the serving provider is not denormalized onto the record; and the
*translated* outbound request and the *raw* upstream response are never captured
(only the client request and an ambiguous response blob are). Second, the
`core.Recorder` interface already carries the comment *"Second implementation:
sqlite"* (`internal/core/interfaces.go:62`) — the architecture anticipated a
queryable store behind the existing write contract.

## What Changes

- **Session history moves to SQLite (`modernc.org/sqlite`, pure Go).** A new
  `internal/history/sqlite/` store implements `core.Recorder`, writing one thin
  indexed row per request. Bodies stay in the existing content-addressed
  `BlobStore` (unchanged); SQLite holds only metadata and `sha256:` blob refs, so
  rows stay uniformly small with no page-bloat penalty.
- **BREAKING — on-disk format changes.** `requests.jsonl` is replaced by a SQLite
  database whose path comes from a new `TINYROUTE_HISTORY_DB` env var (following
  the existing `TINYROUTE_` pattern in `internal/config/service.go`). Existing
  JSONL files are not read; a one-shot importer is a deferred follow-on, not part
  of this change.
- **The JSONL `Recorder` is retired.** SQLite becomes the sole store. The
  `core.Recorder` contract is unchanged, so the proxy still records through it
  (D14 intact). The live per-request tail that JSONL's `tail -f` provided is
  already owned by the `access-logging` capability (`slog` access line emitted at
  completion), so nothing observably is lost.
- **A new read interface supports query.** A `Querier` (history-package-local,
  consumed by the future `history list` CLI command) provides paginated listing
  filtered by provider and start/end timestamp, backed by `(provider, ts)` and
  `(ts)` indexes.
- **Capture gaps are filled.** `core.Usage` gains `CacheCreationTokens`;
  `core.RequestRecord` gains end-to-end `Latency` and a denormalized serving
  `Provider`; and `recordOutcome` captures the translated provider request and
  the raw upstream response as additional blob refs, alongside the clarified
  client-request and client-response refs. The expanded capture remains gated
  behind `TINYROUTE_CAPTURE=full` and is recorded on the winning hop only.

### Non-goals (recorded so they are not relitigated)

- **No aggregation / billing views.** This change adds queryable storage, not
  rollups (cost, daily totals, per-key summaries). Those build on top later.
- **No migration of existing JSONL data.** Young project; start fresh. An
  importer is a deferred follow-on.
- **Bodies stay content-addressed, not in SQLite.** Large payloads never enter
  the database; only `sha256:` refs do.
- **No change to the proxy's D14 boundary.** The proxy still imports only
  `core` + stdlib; it never learns that the store is SQLite.
- **`access-logging` is not modified.** It correlates to history by `request_id`
  unchanged; its requirements do not change.

## Capabilities

### New Capabilities

- `session-history`: tinyroute SHALL persist one record per proxied request in an
  indexed store queryable by provider and timestamp range with pagination, SHALL
  capture billable usage (including cache-creation tokens), end-to-end latency,
  and the serving provider, and SHALL retain request/response payloads in the
  content-addressed blob store behind a capture-mode gate. (Recording previously
  had no formal spec; this codifies it.)

### Modified Capabilities

_None. `access-logging` is referenced for `request_id` correlation but its
requirements are unchanged._

## Impact

- **Code**: new `internal/history/sqlite/` (`db.go` lifecycle + migrations,
  `store.go` `Recorder` impl, `query.go` `Querier` impl); `internal/history/`
  restructured to a parent package holding the `sqlite/` impl plus the existing
  `blobstore.go` and `session.go`, with the JSONL `recorder.go` removed;
  `internal/core/types.go` (`Usage` gains `CacheCreationTokens`, `RequestRecord`
  gains `Latency` and serving `Provider`); `internal/core/interfaces.go` (drop the
  stale "Second implementation: sqlite" comment); `internal/proxy/proxy.go`
  (`recordOutcome` captures latency, serving provider, and the translated/raw
  blobs); `internal/cli/serve.go` (construct + wire the SQLite recorder, read the
  DB path — disjoint region from the in-flight `models-endpoint-conformance`
  `/v1/models` edits); `internal/config/service.go` (`TINYROUTE_HISTORY_DB` +
  `known`-map entry). Dialect usage scanners (`internal/dialect/*`) extended to
  surface cache-creation tokens.
- **Behavior change**: per-request history is queryable with pagination and
  provider/date filters; JSONL output is removed; full-capture mode now also
  archives the translated request and the raw upstream response.
- **Dependencies**: `modernc.org/sqlite` (pure Go, no CGo) added.
- **Migration**: single release; no automatic data migration. Rollback is
  `git revert` (returns to JSONL until re-applied).