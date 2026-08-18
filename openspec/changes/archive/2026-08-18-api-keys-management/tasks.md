# API Keys Management — Tasks

## 1. Auth model simplification (scopes removal)

- [x] 1.1 Delete `Allow` field, `matchesScope`, and `matchGlob` from `internal/auth/keystore.go`; remove the scope check from `Verify` and slim its signature to `Verify(token string) (string, error)`
- [x] 1.2 Update the `Verify` call site in `internal/cli/serve.go` and remove now-unused scope tests from `internal/auth/keystore_test.go`; confirm `go build ./...` passes
- [x] 1.3 Update the `auth` package doc comment (currently mentions scoping) to reflect digest-plus-secret storage without scopes

## 2. Shared mutation helpers in internal/auth

- [x] 2.1 Extend `GenerateKey` with functional options for `Expires *time.Time` and `Rate *RateSpec` (e.g. `WithExpires`, `WithRate`), preserving the existing two-value call form
- [x] 2.2 Add `UpdateKey(path, id string, upd KeyUpdate) error` — loads the file, applies name/expiry/rate changes (secret and digest untouched), writes via `WriteKeyFile`; error when the id is unknown
- [x] 2.3 Add `RevokeKey(path, id string) error` — sets `Disabled`, writes via `WriteKeyFile`; error when the id is unknown or already disabled
- [x] 2.4 Validate inputs in the helpers: expiry must be in the future when set, rate interval must parse with `time.ParseDuration` and requests must be positive
- [x] 2.5 Unit tests: options applied to generated keys, update preserves digest/secret, revoke flips `Disabled`, unknown-id errors, invalid expiry/rate rejected; stale `allow` entries in a fixture file are dropped after any mutation rewrites it

## 3. CLI keys commands

- [x] 3.1 Add `--expires` (duration like `168h` or RFC3339) and `--rate` (e.g. `60/1m`) flags to `keys create`, converting to absolute time before calling the shared helpers
- [x] 3.2 Filter disabled keys out of `keys list` output
- [x] 3.3 CLI tests: create with flags persists expiry/rate, list hides revoked keys, revoke still confirms and persists

## 4. Client installer

- [x] 4.1 Remove the `keyRecord.Allow = []string{dialect + ":*"}` line from `MintKey` in `internal/clients/installer.go`
- [x] 4.2 Update installer tests that assert dialect scoping on minted keys

## 5. Dashboard backend

- [x] 5.1 Add a keys mutation mutex to `DashboardHandler`; all key write handlers hold it across load→mutate→write
- [x] 5.2 Register `POST /dashboard/keys/create`, `POST /dashboard/keys/{id}/update`, `POST /dashboard/keys/{id}/revoke` on `protectedMux`, following the 303-redirect-with-`?error=` convention
- [x] 5.3 Implement the three handlers using the shared helpers: create (name required; optional expiry preset/custom; optional rate), update (name/expiry/rate only), revoke (permanent)
- [x] 5.4 `handleKeysView`: skip disabled keys when assembling rows; derive binary status (`active`/`inactive` from disabled+expiry); include `Secret` in page data for active keys for in-place reveal; keep last-use from history
- [x] 5.5 Fix the clients editor "Provide Key" dropdown to exclude disabled keys alongside the existing `Secret == ""` filter
- [x] 5.6 Handler tests: routes require a session; create/revoke round-trip through the watcher; revoked keys absent from view data; revoked keys never carry secrets into page data; dropdown excludes disabled keys; malformed input redirects with error

## 6. Dashboard UI

- [x] 6.1 Rework `view_keys.templ`: columns Name / Key ID / Secret / Rate / Expires / Status / Actions; binary status badge; **+ Create Key** header button; empty state gains a create affordance
- [x] 6.2 Create dialog: name (required), expiry chips (never/7d/30d) plus custom date input, rate (count + interval); on success show the plaintext once with copy control and the client environment snippet
- [x] 6.3 Edit dialog: same fields pre-filled per key; secret not displayed or editable
- [x] 6.4 Revoke action with confirmation dialog; row disappears on success
- [x] 6.5 Reveal control: masked by default, click to unmask with copy, active keys only; icons via the templ `icon` component (SVG, no emoji)
- [x] 6.6 Render `view_keys_templ.go` (`templ generate`), wire component scripts, and verify pages render in a browser session

## 7. Verification

- [x] 7.1 `gofmt -w .`, `go build ./...`, `go test ./...` all clean; grep confirms no references to `Allow`, `matchesScope`, or `matchGlob` remain outside `openspec/`
- [x] 7.2 Reproducible end-to-end verification in `TestKeysLifecycleEndToEnd` (`internal/cli/keys_e2e_test.go`): create (with and without expiry/rate) → list → reveal → edit rate → revoke → confirm hidden from list and rejected on the request path, all without restarting the daemon
- [x] 7.3 Coverage for `internal/auth` (97.5%) and new dashboard key handlers (`handleKeysView`: 92.3%, `handleKeyCreate`: 92.0%, `handleKeyUpdate`: 94.0%, `handleKeyRevoke`: 100.0%) exceeds the project's 80% bar
