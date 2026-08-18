# API Keys Management

## Why

The dashboard's API Keys page is read-only: keys can be viewed but not created,
edited, or revoked from the UI, so every lifecycle operation falls back to the
CLI or to hand-editing `keys.json`. At the same time the `Key` model carries
fields no surface can set (expiry, rate), while its scope machinery
(`Allow`, `matchesScope`, `matchGlob`) adds a concept users must understand for
a benefit that does not justify its weight in a single-admin gateway. This
change makes the keys menu a complete management surface and simplifies the key
model to the irreducible core: mint, see, limit, kill.

## What Changes

- **Keys menu becomes a management surface** with Create, Edit, Revoke, and
  Reveal actions:
  - **Create** dialog: name (required), expiry (optional: never / 7d / 30d /
    custom), rate (optional: requests per interval). The plaintext is shown
    once at creation with a copy control.
  - **Edit** dialog: same three fields, pre-filled; changes constraints
    without rotating the credential. The secret is never editable.
  - **Revoke**: behind a confirm dialog; permanent in the UI (no enable path).
    Revoked keys are hidden from the dashboard list and from `keys list`;
    verification continues to reject them on every request.
  - **Reveal**: active keys only — masked by default, click to unmask and
    copy. Revoked keys cannot be revealed (their rows do not render).
- **Status is binary**: `active` or `inactive` (inactive = expired). No
  per-state badges beyond those two.
- **BREAKING — scopes are removed entirely.** `Key.Allow`, `matchesScope`,
  `matchGlob`, and the scope check in `Verify` are deleted;
  `Verify(token, surface, model)` becomes `Verify(token)`. The client
  installer stops pinning minted keys to a dialect. Consequence (accepted):
  existing keys with `allow` entries in `keys.json` become full-access once
  the new build parses the file; stale `allow` entries disappear from the file
  on the next mutation, since writes re-marshal the struct.
- **Plaintext persistence is formalized.** The keystore already stores
  `Key.Secret` so keys can be re-embedded into client configs; the specs
  currently (and incorrectly) mandate digest-only storage. This change aligns
  the specs with the code and with the reveal feature. Secrets remain masked
  in all list output, never appear in URLs, flash parameters, or logs, and
  `keys.json` keeps mode `0600` atomic writes.
- **Shared model-layer helpers** in `internal/auth` are consumed by both the
  dashboard handlers and the CLI, which gains `keys create --expires` and
  `--rate` flags.
- **Bug fix folded in**: the clients page "Provide Key" dropdown currently
  offers disabled keys; it SHALL exclude them.

## Capabilities

### New Capabilities

(none — all affected behavior belongs to existing capabilities)

### Modified Capabilities

- `api-keys`: Scope requirements are removed; minting is no longer
  CLI-exclusive (dashboard can create/reveal); digest-only storage is replaced
  by digest-plus-persisted-secret; revocation gains hidden-from-lists
  semantics; expiry and rate become settable from surfaces.
- `management-dashboard`: The keys view graduates from observe-only to a
  management surface with mutations; the observe-views requirement is relaxed
  for keys; the client-apply mint flow drops dialect scoping; the "Provide
  Key" dropdown must exclude disabled keys.
- `clients-install`: Minted keys are no longer dialect-scoped (`Allow` is
  gone); the mint flow otherwise behaves as before (plaintext revealed once,
  written into the agent config).

## Impact

- **Code**:
  - `internal/auth/keystore.go` — delete `Allow`/`matchesScope`/`matchGlob`,
    slim `Verify`, add shared create/update helpers (`GenerateKey` opts,
    revoke)
  - `internal/cli/serve.go` — `Verify` call site
  - `internal/cli/commands.go` — `keys create` flags, `keys list` filters
    revoked
  - `internal/clients/installer.go` — drop dialect pinning in `MintKey`
  - `internal/dashboard/handler.go` — new POST routes
    (`keys/create`, `keys/{id}/update`, `keys/{id}/revoke`), list filter,
    dropdown fix
  - `internal/dashboard/view_keys.templ` — actions, dialogs, status badge,
    secret reveal
- **Data**: `keys.json` — `allow` entries ignored on read, dropped on next
  write; no migration step
- **Security**: existing scoped client keys become full-access (accepted);
  secrets remain masked in output and excluded from logs/URLs; revoke is
  irreversible from the UI, recoverable only by editing `keys.json`
- **Specs**: `api-keys`, `management-dashboard`, `clients-install` delta specs
  in this change
