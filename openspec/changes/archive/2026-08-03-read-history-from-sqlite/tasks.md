## 1. Query layer: key + session filters

- [x] 1.1 Add `KeyID` and `Session` fields to `history.Filter` (`internal/history/query.go`)
- [x] 1.2 Add `WHERE key_id = ?` and `WHERE session = ?` clauses to `Store.List` (`internal/history/sqlite/query.go`)
- [x] 1.3 Tests: filtering by key, by session, and combined with provider/timestamp/pagination

## 2. Query layer: per-key last-use

- [x] 2.1 Add a query returning the most recent timestamp per `key_id` (e.g. `SELECT key_id, MAX(timestamp) FROM requests WHERE key_id != '' GROUP BY key_id`)
- [x] 2.2 Expose it on the store/Querier (e.g. `LastUseByKey(ctx) (map[string]time.Time, error)`)
- [x] 2.3 Tests: last-use per key; keys with no usage absent or "never"

## 3. Rewire CLI history reads to SQLite

- [x] 3.1 `cmdSessions` — read via `Store.List`, group by `Session`, add a `KEY` column (distinct keys per session); honor `--key` via `Filter.KeyID`
- [x] 3.2 `cmdSessionReplay` — read via `Store.List` with `Filter.Session`; print `key_id` per turn
- [x] 3.3 `cmdLog` — render recent records from `Store.List` instead of tailing `requests.jsonl`
- [x] 3.4 `deriveLastUse` — switch from `readHistory(jsonl)` to `LastUseByKey`; update `keys list` LAST USE
- [x] 3.5 Open the SQLite store once per command (`sqlite.Open(svc.HistoryDBPath)` → `sqlite.NewStore`)

## 4. Remove the dead JSONL read path

- [x] 4.1 Delete `readHistory` and its `requests.jsonl` callers in `internal/cli/commands.go`
- [x] 4.2 Decide `TINYROUTE_HISTORY` fate: deprecate (no-op alias) or remove the `HistoryPath` field — record the choice in `design.md` Open Questions

## 5. Verification

- [x] 5.1 With `tinyroute serve` recording traffic: `sessions`, `session <id>`, `log`, and `keys list` LAST USE all reflect recorded data
- [x] 5.2 `sessions --key <id>` filters server-side; replay of a real session shows turns + key
- [x] 5.3 `gofmt -w .`; reach 80% coverage on changed files; `go test ./internal/history/... ./internal/cli/...`
