## Context

Per-request recording today is `history.Recorder` (`internal/history/recorder.go`),
which appends one JSONL line per proxied request to `requests.jsonl` from an
off-critical-path goroutine in `proxy.recordOutcome`
(`internal/proxy/proxy.go:264`). Bodies, when `TINYROUTE_CAPTURE=full`, are
stored separately in the content-addressed `history.BlobStore`
(`internal/history/blobstore.go`) and referenced from the line by `sha256:`
digests. The `core.Recorder` interface (`internal/core/interfaces.go:60`) is the
contract the proxy writes through; it carries the comment *"Second implementation:
sqlite."*

Two constraints inherited from the codebase shape this design:

- **D14 — the proxy imports only `core` + stdlib** (`internal/proxy/proxy.go:1`).
  The proxy must never learn that the store is SQLite.
- **The BlobStore is the payloads' home.** Bodies are already factored out and
  content-addressed; the line/row only ever holds `sha256:` refs.

The trigger for the change is the near-term usage surface: list requests,
paginated, filtered by provider and a start/end timestamp — a workload JSONL
serves only with O(n) full-file scans and no index. Recording also has gaps the
new surface exposes: no cache-creation tokens, no end-to-end latency, no
denormalized serving provider, and no capture of the translated outbound request
or the raw upstream response. See `proposal.md` for the full motivation and
non-goals.

## Goals / Non-Goals

**Goals:**

- One indexed row per proxied request, queryable by `(provider, ts)` and `(ts)`
  with cursor pagination.
- Fill the four capture gaps (cache-creation, end-to-end latency, serving
  provider, translated-req / raw-resp blobs) while keeping bodies
  content-addressed.
- D14 intact: `internal/proxy` gains no new import.
- JSONL retired with no observable loss (the `access-logging` `slog` line already
  owns the live tail).
- Single-binary build preserved (no CGo).

**Non-Goals:** (mirror `proposal.md`) no aggregation/billing views; no JSONL
data migration; bodies never enter SQLite; `access-logging` requirements
unchanged.

## Decisions

### D1. Driver: `modernc.org/sqlite` (pure Go), not CGo

The store uses `modernc.org/sqlite`, a pure-Go (transpiled-from-C) SQLite. No
CGo, so `go build` cross-compiles and produces a static single binary exactly as
today — a hard requirement for a gateway shipped as one artifact. The per-request
write rate here is low (one row per inference request, off the critical path),
so modernc's modest speed disadvantage vs `mattn/go-sqlite3` is irrelevant; the
build-simplicity advantage is decisive.

*Alternatives rejected:* `mattn/go-sqlite3` (CGo — faster, but a build/cross-compile
tax for no workload benefit); a custom file index over JSONL (re-invents SQLite,
badly).

### D2. Single thin table; bodies stay content-addressed

One `requests` table holds metadata, usage, latency, and nullable `sha256:` blob
refs. Because payloads live in the `BlobStore`, rows are uniformly small — there
is no page-density penalty to keeping everything in one table, so no
usage-vs-payload split is warranted. `attempts[]` is stored as a JSON text column
(per-hop forensic data, rarely filtered); it is not normalized into a child table
until a per-hop analytics requirement actually appears (YAGNI).

```sql
CREATE TABLE requests (
  id            TEXT PRIMARY KEY,          -- == access-log request_id
  ts            INTEGER NOT NULL,          -- unix ms (range/sort index)
  provider      TEXT,                      -- serving hop, denormalized
  model_req     TEXT NOT NULL,
  model_served  TEXT,
  key_id        TEXT,
  session       TEXT,
  endpoint      TEXT,
  stream        INTEGER NOT NULL,
  outcome       TEXT NOT NULL,
  input_tokens          INTEGER NOT NULL DEFAULT 0,
  output_tokens         INTEGER NOT NULL DEFAULT 0,
  cache_read_tokens     INTEGER NOT NULL DEFAULT 0,
  cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
  latency_ms    INTEGER,
  req_blob         TEXT,                   -- raw client request
  xlated_req_blob  TEXT,                   -- translated provider request
  raw_resp_blob    TEXT,                   -- raw upstream response
  client_resp_blob TEXT,                   -- client-facing response
  attempts      TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX idx_requests_ts       ON requests(ts DESC);
CREATE INDEX idx_requests_provider ON requests(provider, ts DESC);
CREATE INDEX idx_requests_session  ON requests(session, ts DESC);
```

`ts` is `INTEGER` unix-millis, not RFC3339 text: range scans and ordering are
index-cheap with zero parse cost (the JSONL `time.Time` becomes millis at the
boundary). `provider` and `latency_ms` are real columns, not derived — that
denormalization *is* the reason to leave JSONL: the two future filters
(`provider`, `from/to`) become index seeks instead of full-file scans.

### D3. Concurrency: WAL + a shared `*sql.DB`, recorder mutex removed

The recorder's existing `sync.Mutex` (JSONL file-append guard) is **removed**.
SQLite serializes writes itself; a single process-wide `*sql.DB` (opened once in
`serve.go`, shared by the recorder and the CLI query path) pools connections, and
`PRAGMA journal_mode=WAL` plus `busy_timeout=5000` let CLI readers paginate while
the proxy writes concurrently. `PRAGMA synchronous=NORMAL` is safe under WAL and
keeps the off-path write cheap.

### D4. Migration tracking via `PRAGMA user_version`

Schema is versioned with `PRAGMA user_version` (an integer the driver reads/writes
directly) and a slice of ordered `CREATE ... IF NOT EXISTS` statements applied in
`sqlite/db.go`. No third-party migration library — the project is stdlib-plus-one
(`database/sql` + modernc); a migration framework is unjustified weight at this
schema size. `Migrate` is idempotent and runs at `Open`.

### D5. `Querier` is history-package-local, not in `core`

The read interface lives at the `internal/history` package level (a new
`query.go`), consumed by the future `history list` CLI command. It is **not**
added to `core`: only the CLI reads, and the proxy never queries (recording is
write-only from its side). This honors the coding-style rule "define interfaces
where consumed" and keeps `core` from growing a read surface no current consumer
needs. If a second consumer appears, promote it then.

Shape:

```
List(ctx, filter{
    Provider string // "" = all
    From, To int64  // unix ms, inclusive; 0 = unbounded
    Limit    int
    Cursor   string // opaque, derived from (ts, id) of the last row
}) (rows []Summary, nextCursor string, err error)
```

`Summary` is the thin row minus the blob refs; full bodies are fetched on demand
via the existing `BlobStore.Get` by the CLI, not by `List`.

### D6. Capture scope: winning hop only, still gated behind `TINYROUTE_CAPTURE=full`

The translated-request and raw-response blobs are captured for the **winning hop
only** (the attempt that committed a `2xx`), not for every retried hop. This
bounds the storage/PII expansion to at most four blob writes per request (client
req, translated req, raw resp, client resp) and keeps the capture behind the
existing `TINYROUTE_CAPTURE=full` gate with its PII warning
(`internal/history/recorder.go:25`). Per-hop full capture for failed attempts is
deferred (forensic use only; the `attempts[]` JSON already records per-hop status
and elapsed).

*Serving provider derivation:* the last attempt with a `2xx` status. For
non-OK outcomes (`chain_exhausted`, `no_route`, `auth_failed`) `provider` is
empty — the row records that no provider served the request, which is correct.

### D7. End-to-end latency is proxy-stamped, not joined from access-log

The proxy stamps `start := time.Now()` at the top of `Handler` and passes
`time.Since(start)` into `recordOutcome`, which stores `latency_ms`. This is
self-contained within the proxy/Deps and needs no context-writeback from the
access-log middleware. The access-log's own `time.Since` (middleware-level, wider
scope) is unaffected and remains the transport-latency source of truth; the
history `latency_ms` is the proxy-path latency specifically. Joining the two by
`request_id` is still possible when both are wanted.

### D8. D14 preserved: `core.Recorder` unchanged, SQLite invisible to the proxy

`core.Recorder` and `core.RequestRecord` are the only surfaces the proxy sees.
The proxy's edits are: stamp start time, derive serving provider, capture the
translated/raw bodies for the winning hop, and pass an enriched `RequestRecord`
through the **unchanged** `Recorder.Record`. The SQLite implementation, the
`*sql.DB`, and `modernc` are all confined to `internal/history/sqlite` + `serve.go`
construction. The proxy compiles with zero knowledge of the engine.

### D9. JSONL retired; `access-logging` owns the live tail

The JSONL `Recorder` (`recorder.go`) is deleted; `internal/history` becomes a
parent package containing the `sqlite/` implementation plus the unchanged
`blobstore.go` and `session.go`. The `tail -f requests.jsonl` debug affordance is
already replaced by the `access-logging` per-request `slog` line (a different,
structured, live view), so the removal loses nothing operators rely on. The stale
`// Second implementation: sqlite.` comment on `core.Recorder` is removed.

## Risks / Trade-offs

- **[On-disk format break — `requests.jsonl` unreadable]** → Acceptable for a
  young project with no external consumer of that file. *Mitigation:* the
  `BREAKING` marker in the proposal; a deferred one-shot importer if any operator
  has data worth keeping.
- **[New dependency — `modernc.org/sqlite`]** → First non-stdlib storage dep.
  *Mitigation:* pure Go (no CGo), widely used, transpiled from upstream SQLite so
  SQL semantics are canonical; confined to `history/sqlite`.
- **[modernc slower than CGo]** → Irrelevant at one write per inference request
  off the critical path. *Mitigation:* WAL + `synchronous=NORMAL`; revisit only
  if write throughput becomes measurable.
- **[Capture expansion — up to 4 blobs/request]** → More disk + PII surface.
  *Mitigation:* winning-hop-only; gated behind `TINYROUTE_CAPTURE=full`; existing
  PII warning retained; `BlobStore` dedup keeps repeat payloads cheap.
- **[Concurrent CLI read during proxy write]** → Read/write contention.
  *Mitigation:* WAL lets readers proceed without blocking the writer;
  `busy_timeout` absorbs transient lock waits.
- **[`attempts` as JSON column — unqueryable per-hop]** → Per-hop analytics
  requires scanning rows. *Mitigation:* accepted for now (forensic, rarely
  filtered); normalize to a child table if/when per-hop queries become real.
- **[Migration ordering vs `models-endpoint-conformance`]** → Both touch
  `serve.go`. *Mitigation:* disjoint regions (recorder construction vs `/v1/models`
  handler); trivial merge in either order. Verified by grep — no recorder/history
  references in that change.

## Migration Plan

Single release. No data migration. `serve.go` constructs the SQLite store at
startup (open DB at `TINYROUTE_HISTORY_DB`, set PRAGMAs, run `Migrate`), injects
it as the `core.Recorder` (and `Querier` for the CLI), and closes it on shutdown.
Old `requests.jsonl` files are simply ignored. Rollback is `git revert`, which
restores the JSONL recorder; the SQLite DB file can be deleted.

## Open Questions

- **`TINYROUTE_HISTORY_DB` default location.** Alongside today's data dir (e.g.
  `~/.tinyroute/history.db`), or a `data/` sibling? Lean alongside the existing
  history path so operators find it in the same place. Resolve at task time.
- **Retention/rotation.** JSONL rotated at `maxBytes`; SQLite has no analog here.
  A row-count or age-based retention job is out of scope but worth a follow-on
  issue so the DB doesn't grow unbounded. Not blocking.
- **`history list` CLI shape.** Pagination cursor encoding (opaque base64 of
  `ts:id`?) and flag surface (`--provider`, `--from`, `--to`, `--limit`) are
  decided during implementation per the `cli-interactivity` rules (non-TTY +
  missing filter → clear error; large result sets stay usable).