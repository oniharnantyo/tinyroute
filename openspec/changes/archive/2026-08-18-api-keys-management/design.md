# API Keys Management — Design

## Context

The keys menu (`internal/dashboard/view_keys.templ` + `handleKeysView`) renders
a read-only table from the hot-reloaded `KeyStore` watcher. All mutation
machinery already exists at the model layer (`auth.GenerateKey`,
`auth.WriteKeyFile` — atomic, `0600`, picked up by the mtime watcher) and the
dashboard has an established mutation pattern (`POST /dashboard/<thing>/<verb>`
→ mutate → `303` redirect with `?error=`). The CLI owns the only key lifecycle
commands today (`keys create/list/revoke`). Two spec/code inconsistencies
motivate parts of this design: the `api-keys` spec mandates digest-only
storage while the code deliberately persists `Key.Secret` for re-embedding, and
the scope layer (`Allow`) is load-bearing only for the client installer's
dialect pinning — a trade the owner has accepted to drop.

## Goals / Non-Goals

**Goals:**

- Keys page: Create (name + optional expiry + optional rate), Edit (same
  fields, pre-filled), Revoke (confirm, permanent), Reveal (active keys only)
- Binary status badge: `active` / `inactive` (inactive = expired)
- Revoked keys hidden from dashboard list and `keys list`
- Remove the scope layer end-to-end; slim `Verify` to `(token)`
- Shared create/update/revoke helpers in `internal/auth` for both surfaces
- CLI gains `keys create --expires` / `--rate`
- "Provide Key" dropdown on the clients page excludes disabled keys

**Non-Goals:**

- Key deletion (records persist forever; revoke only)
- Un-revoke / enable path in any surface
- Scope management (feature removed, not made manageable)
- Multi-admin concurrency control across processes
- New JSON API for keys (server-rendered pages + form POSTs only)

## Decisions

### D1: `Verify` slims to `Verify(token)`

With scopes gone, `surface` and `model` are dead parameters. Remove them
rather than keeping them "in case scopes return" — unused parameters compile
silently in Go, and a truthful signature is the discipline that makes the
removal real. Single call site (`internal/cli/serve.go:412`) adjusts; the
dialect variable `d` remains in use for `ParseRequest`/`WriteError`.

### D2: Scope removal needs no migration

`json.Unmarshal` ignores unknown fields, so existing `"allow"` entries in
`keys.json` are silently skipped on read — those keys become full-access.
Because mutations re-marshal the struct (`ParseKeyFile`-style load → mutate →
`WriteKeyFile`), the next write of the file drops stale entries permanently.
No migration step, no schema version. Rollback caveat noted in Risks.

### D3: Reveal = secret embedded in page data, client-side unmask

`handleKeysView` includes each active key's `Secret` in `KeysPageData`; the
template renders it masked in the DOM with a reveal/copy toggle. Alternative —
a fetch-on-demand `POST /keys/{id}/reveal` endpoint — was rejected: the page is
already password-authenticated behind `protectedMux`, the clients page already
ships plaintexts into configs, and an extra route adds CSRF surface for no
gain. Constraints preserved: plaintext never in URLs, flash/query parameters,
or logs; revoked keys render no row, hence no secret.

### D4: Mutations follow the existing POST→303 pattern, under existing CSRF protection

New routes: `POST /dashboard/keys/create`, `POST /dashboard/keys/{id}/update`,
`POST /dashboard/keys/{id}/revoke` — registered on `protectedMux`, inheriting
the dashboard's cross-origin protection and session auth like every other
mutating route. On success redirect to `/dashboard/keys`; on failure redirect
with `?error=<message>`. This matches `providers/add`, `models/add`, etc.

### D5: Shared mutation helpers in `internal/auth`

Formalize the load→mutate→write cycle both surfaces duplicate:

- `GenerateKey(name string, opts ...KeyOpt)` — extends the existing signature
  with optional `Expires *time.Time` and `Rate *RateSpec`
- `RevokeKey(path, id string) error` / `UpdateKey(path, id string, upd KeyUpdate) error`
  — encapsulate read, mutate, `WriteKeyFile`

Validation (interval parsing via `time.ParseDuration`, expiry in the future)
lives in these helpers so the CLI and dashboard cannot drift. Dashboard
mutations serialize under a dedicated keys mutex (or `h.mu`) to prevent two
concurrent dashboard writes clobbering each other; CLI-vs-dashboard races
remain last-writer-wins (accepted, single-admin tool).

### D6: Surfaces convert human input; the model stores absolutes

`Key.Expires` stays `*time.Time` (absolute). The dashboard offers preset chips
(never / 7d / 30d) plus a custom `datetime-local` input; the CLI `--expires`
accepts a duration (`168h`) or RFC3339 timestamp, converted before reaching
the helper. `Rate` stays `{Requests int, Interval string}` with
`time.ParseDuration`-valid intervals.

### D7: Status is derived, not stored

`active` = `!Disabled && (Expires == nil || now.Before(*Expires))`; everything
else renders `inactive`. The badge is computed at render time — no new field on
`Key`, nothing to keep in sync on write.

### D8: Revoked filtering happens server-side, at the data assembly step

`handleKeysView` and `cmdKeysList` skip `Disabled` keys before building rows.
`keys.json` remains the unfiltered source of truth (the only recovery path for
a revoked key). The clients page "Provide Key" dropdown gains the same
`k.Disabled` check next to its existing `k.Secret == ""` filter.

## Risks / Trade-offs

- [Existing scoped client keys become full-access on upgrade] → Accepted by
  owner during exploration; documented as BREAKING in the proposal. Users who
  relied on dialect pinning must rotate client keys they consider exposed.
- [Rollback cannot restore `allow` entries] → Once any mutation rewrites
  `keys.json`, dropped scopes are unrecoverable; mitigation: rotate/rotate-back
  is a re-add, not a restore. Communicate in release notes.
- [Secrets present in page HTML] → View-source/screen exposure on an
  authenticated page; mitigated by mask-in-DOM default, copy control, and no
  revoked-key rows. Consistent with existing client-apply reveal-once flow.
- [Concurrent dashboard + CLI writes race] → Last-writer-wins; accepted for a
  single-admin local tool; in-process mutex removes the more likely
  two-tabs-case.
- [`Verify` signature change breaks callers] → Compile-time failure; single
  call site in tree.

## Open Questions

None — all decisions were resolved during exploration (2026-08-18). The
401-vs-403 question for scope denials is moot: scope denials no longer exist.
