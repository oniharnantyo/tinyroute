## Why

The request-history recorder writes to **SQLite** (`history.db`, via `history/sqlite/store.go`
wired in `serve.go`), but the CLI history views still read the legacy `requests.jsonl` — a file
nothing has written since the SQLite migration. Those views are therefore silently empty/stale:

- `tinyroute sessions` → "no sessions recorded yet"
- `tinyroute session <id>` (replay) → finds nothing
- `tinyroute log` → empty
- `tinyroute keys list` → `LAST USE` shows "never" for every key

…despite traffic being recorded. A complete SQLite query layer already exists
(`history.Querier.List` → `[]history.Summary`, which already returns `key_id`) but is unused by
the CLI. This change rewires the CLI to that layer and surfaces the key identifier.

## What Changes

- Rewire every CLI history read to the SQLite store via `history.Querier` (`Store.List`):
  `cmdLog`, `cmdSessions`, `cmdSessionReplay`, and `deriveLastUse` (the `keys list` last-use
  column) in `internal/cli/commands.go`.
- Extend `history.Filter` with `KeyID` and `Session` dimensions (with SQLite `WHERE` clauses) so
  the existing `sessions --key` filter and session replay work server-side with correct
  pagination.
- Surface the authenticating key identifier (`key_id`) in the `sessions` table and in session
  replay.
- Derive `keys list` `LAST USE` from recorded history (most recent record per key).
- Remove the dead JSONL read path (`readHistory` / `deriveLastUse` over `requests.jsonl`).

No **BREAKING** changes to routes, the recorder, or the on-disk SQLite schema — the `key_id`
column already exists.

## Capabilities

### Modified Capabilities

- `session-history`: the query layer gains key + session filter dimensions, and history views are
  required to read from the recorded store (not a legacy log).

### New Capabilities

None.

## Impact

- `internal/cli/commands.go` — `cmdLog`, `cmdSessions`, `cmdSessionReplay`, `deriveLastUse`
  switched to `history.Querier`; `KEY` column added to `sessions`.
- `internal/history/query.go` — `Filter` gains `KeyID`, `Session`.
- `internal/history/sqlite/query.go` — `WHERE` clauses for key/session; likely a per-key
  last-timestamp query.
- `internal/history/*_test.go` — coverage for the new filters and read paths.
