## Context

Recording was migrated to SQLite (`history/sqlite/store.go` implements `core.Recorder`, wired in
`serve.go`), but the CLI read paths were left on the pre-migration JSONL (`requests.jsonl`). The
JSONL writer was removed during the migration, so `requests.jsonl` is write-orphaned; every CLI
reader of it (`cmdLog`, `cmdSessions`, `cmdSessionReplay`, `deriveLastUse`) sees an empty/stale
file. A full SQLite query layer already exists — `history.Querier.List(ctx, Filter)` returns
`[]history.Summary` carrying `KeyID`, `Session`, `Provider`, tokens, latency, etc. — but no CLI
command calls it.

## Goals / Non-Goals

**Goals:**
- CLI history views reflect recorded traffic (sessions, replay, log, keys-list last-use).
- Surface the authenticating key id in history views.
- One read path (the recorder's store) — eliminate the read/write divergence.

**Non-Goals:**
- Changing the recorder or the SQLite schema (`key_id` already exists).
- Importing legacy JSONL data (none exists post-migration).
- New history UIs or changing the recorded fields.

## Decisions

### D1. Single read path via `history.Querier`
All CLI history reads go through `Store.List` (SQLite) — the same store the recorder writes — so
read/write divergence cannot recur. Remove `readHistory(jsonl)` and the `cmdLog` JSONL tail.

### D2. Extend `Filter` with `KeyID` and `Session` (server-side)
The existing `sessions --key` and `session <id>` replay need to filter by key and session. Doing
this client-side after a paginated `List` is lossy (a page may exclude matching rows), so the
filter is pushed into `history.Filter` + the SQLite `WHERE` clause.

### D3. Surface `key_id`
The `sessions` table gains a `KEY` column (the distinct key(s) for the session; sessions rarely
span keys, but the code shows the set when they do), and replay prints the key per turn.

### D4. Last-use per key via a dedicated query
`keys list` LAST-USE needs the most recent timestamp per `key_id`. Aggregating over `List` risks
pagination truncation on busy gateways, so add a small query (`MAX(timestamp) ... GROUP BY key_id`,
or equivalent) consumed by `deriveLastUse`.

### D5. Remove the dead JSONL path
`readHistory` and the `cmdLog` JSONL tail are removed. `TINYROUTE_HISTORY` (the JSONL path) becomes
a deprecated/no-op config alias (see Open Questions) — it no longer represents live data.

*Alternative considered:* dual-write (SQLite + JSONL) to preserve the old readers — rejected; it
reintroduces a second source of truth and defeats the migration.

## Risks / Trade-offs

- **[Filter + pagination correctness]** client-side filtering is lossy → mitigated by server-side
  `KeyID`/`Session` filters (D2).
- **[Last-use over large history]** a per-key scan on every `keys list` is costly → mitigated by a
  single `GROUP BY key_id` query (D4).
- **[Back-comat]** any user with meaningful pre-migration JSONL data loses CLI access to it →
  acceptable: post-migration the file was never written, so it holds no data.
- **[`cmdLog` semantics]** it currently tails the JSONL; repurpose it to render recent SQLite
  records (same intent: "show recent traffic").

## Migration Plan

None — the SQLite store is already populated by the running recorder; switching the readers to it
immediately reflects recorded data. Rollback = revert the CLI reads (the recorder is unaffected).

## Open Questions

- **Keep `TINYROUTE_HISTORY` as a deprecated config alias, or remove the field entirely?**
  *Resolution:* Retained `TINYROUTE_HISTORY` / `HistoryPath` in `Service` struct marked as deprecated to preserve environment configuration backwards compatibility while all CLI commands exclusively query `HistoryDBPath` (`history.db`).
- **Exact `tinyroute log` output format post-rewire (mirror the old tail, or a richer per-record line)?**
  *Resolution:* Mirrored the original output format for `tinyroute log` while sourcing records directly from `history.Querier` in SQLite.
