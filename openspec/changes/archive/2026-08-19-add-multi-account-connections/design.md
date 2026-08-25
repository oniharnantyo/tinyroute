# Design: Add Multi-Account Connections

## Context

See `proposal.md` — Why. Current state in one paragraph: the runtime
(`internal/config/topology.go` `BuildCredentials`, `internal/route` selection +
per-account cooldowns) and the credential store (`internal/credential/store.go`,
keyed `provider/account` with legacy `provider` → `provider/default` fallback) are
fully account-aware. The write paths are not: dashboard `handleProviderCredential`
writes the scalar `provider.api_key` (`handler.go:1043`); dashboard and CLI OAuth
flows save `OAuthRecord` with no `Account`, so `Store.Save` defaults to `default`
and overwrites; `cmdAuthLoginWithAccount` accepts an account name but none of the
four flow runners receive it; `cmdAccountAdd` swallows the delegated flow's error.

Three product decisions were made during exploration (AskUserQuestion, 2026-08-18):

1. **Account naming derives from provider identity** when no explicit label is
   given (email/sub claims from the token response), with an auto-slot fallback.
2. **Static API keys move to `Provider.Accounts[]`**; the scalar `api_key` becomes
   a legacy shim.
3. **Full surface**: dashboard + CLI write paths, topology linkage, per-connection
   reconnect (rotate in place) and rename.

## Goals / Non-Goals

**Goals:**

- One shared, deterministic account-name resolver used by every credential write
  path (CLI flows, CLI account commands, dashboard handlers).
- Credential saves and topology `Accounts[]` linkage happen together.
- Dashboard write affordances match its already multi-account read model.

**Non-Goals:**

- New selection strategies, quota UI, or changes to the runtime account model
  (it already works).
- Migrating existing `provider/default` records or scalar `api_key` values —
  read-path fallbacks already cover them; all changes are additive.
- Verifying token-claim signatures (claims are label material only).

## Decisions

### D1: A resolver in `internal/credential` owns the naming ladder

New `naming.go` in `internal/credential`:

```go
// ResolveAccount picks the store account name for a write.
//   explicit label (validated)  →  token identity hint  →  first free slot
func ResolveAccount(provider, explicit, identityHint string, existing []string) (string, error)
func ValidateAccountName(name string) error   // charset [A-Za-z0-9._@-], no '/', ≤64
func SanitizeDerivedName(s string) string     // lower, strip invalid; "" if unsalvageable
```

Callers pass `existing` = union of the provider's store keys and topology
`Accounts[].Name` (both matter: a topology account with no stored token is still a
taken name). The resolver is pure (no I/O) so it is trivially testable and usable
from both CLI and dashboard.

*Alternatives:* putting it in `internal/config` (couples naming to topology
parsing) or `internal/oauth` (unavailable to static-key writes). The store key is
the thing being named, so `internal/credential` is the natural home.

Collision rule (per spec): an **explicit** name that exists → update in place
(deliberate rotation); a **derived** name that exists → next free slot. Slot
numbering starts at `account-2` (`default` is implicitly the first account).

### D2: Identity capture via `id_token` parsing + best-effort JWT decode, carried as a non-persisted hint

Token-response parsers (`internal/oauth/oauth.go` PKCE/device, the four runners in
`internal/cli/auth.go`) gain an `id_token` field. Derivation: `id_token` payload
`email` → access-token JWT payload `email`/`sub` (only if the access token is
three dot-separated base64url segments). No signature check — the value is
sanitized (`SanitizeDerivedName`) and used only as a label.

The hint travels on `OAuthRecord` as `IdentityHint string `json:"-"`` so flow
runners can return it without changing their signatures twice; `json:"-"` keeps it
out of credentials.json. The resolver consumes it before `Save`.

### D3: CLI threading — the account name becomes a flow-runner parameter

All four runners (`runDeviceCodeFlow`, `runPKCEFlow`, `runQoderFlow`,
`runTraeFlow`) take the resolved-or-explicit account name and set `rec.Account`
before `Save`. When the caller gave no label and the store already holds a
connection for the provider, `cmdAuthLoginWithAccount` runs the resolver *after*
the flow completes (identity hint in hand) instead of pre-picking a slot. Fresh
providers keep today's behavior: no label, no existing connection → `default`.

`cmdAccountAdd` (`--type oauth_refresh`) passes its chosen name as the explicit
label, propagates the flow error, and writes topology only after the flow
succeeds — fixing the swallowed `_ =` at `account.go:301`.

### D4: Dashboard threading — the account label rides the OAuth session, not the URL

`OAuthStateSession` gains an `Account string` field. The connect affordance
becomes a `dialog` (existing shadcn-templ component) with an optional label
`input`; the label posts to `/oauth/start` and is stored in the session, then
applied at callback/poll completion via the same resolver. Reconnect on a
connection row's `dropdown` starts the flow with that row's account as the
explicit label.

Device flow state currently round-trips through redirect query params; the label
joins `device_code`/`device_id` there. That puts a possibly-PII label in a
localhost-only URL — acceptable; noted in Risks.

### D5: `UpsertAccount` on `config.Provider` closes the linkage gap

```go
func (p Provider) UpsertAccount(acc Account) Provider  // append or replace-by-name
```

Every successful credential save follows the same write order: `store.Save` (mode 0600 tmp+rename) → read topology →
`ensureMaterialized` → `p.UpsertAccount(acc)` → `config.WriteTopology`. Credential
failure aborts before topology is touched; topology failure returns an error
clearly stating the credential saved but topology write failed. (Note on store
watcher hot-reloading: `fileWatcher` checks `!info.ModTime().Equal(w.mtime)` and
mutation methods `Save`/`Delete`/`DeleteProvider` invoke `w.reload()` immediately
to ensure atomic file swaps under fast sequential writes reflect in-memory).

### D6: Static keys append `Accounts[]`; scalar is a write-once legacy shim

`handleProviderCredential` resolves a name (explicit label from the dialog input,
else first free slot), appends `Account{Name, Type: "static", APIKey: key}`, and
never touches the scalar. First dashboard add on a scalar-only provider creates
`Accounts[]` — from then on the runtime's existing precedence
(`len(Accounts) > 0` wins, `topology.go:204`) routes through accounts. The scalar
is left intact for rollback; nothing reads it once accounts exist.

### D7: Rename re-keys store + topology in one gesture

New handler: load record under `provider/<old>` → `Save` under `provider/<new>`
(with validated new name; collision rejected) → `Delete` old key → rewrite
`Accounts[].Name` → `WriteTopology`, after a `dialog` confirmation. Cross-file
atomicity is impossible (two files, no transaction manager); store-first ordering
plus idempotent retry is the mitigation, consistent with D5.

### D8: UI surface

Provider detail page only — the Connections panel already lists accounts. Changes:
add-key moves into a `dialog` (secret `input` + optional label `input`); each
connection row's `dropdown` gains Reconnect / Rename / (existing) Disconnect;
feedback is `toast` via `window.tui` (success variant naming the account;
destructive variant for validation failures). No new components needed; no
bespoke widgets. Icons: existing Lucide set (e.g. `refresh-cw`, `pencil`),
no emoji.

## Risks / Trade-offs

- [Emails land in topology.yaml and store keys — mild PII in a non-secret file]
  → Accepted product decision (D1/D2). Account names are labels only; masking
    rules for secrets are unchanged. Documented in the README section for
    multi-account.
- [JWT claims decoded without signature verification] → Claims are sanitized and
  used only as names; they never influence routing or authorization. A malicious
  provider could at worst choose its own account label.
- [Two-file writes are not atomic (D5/D7)] → Store-first ordering, idempotent
  retry, errors state exactly which half succeeded. Local single-user tool;
  a real transaction manager is out of scope.
- [Device-flow label appears in redirect URLs] → Dashboard binds to localhost;
  the label is a name, not a secret. If this ever bothers anyone, move device
  flow state server-side like PKCE already is.
- [`account-2`-style names are opaque] → Rename exists (D7) and the connect
  dialog offers an explicit label; auto-slot is the floor, not the ceiling.
- [Providers that rotate refresh tokens on every use] → Reconnect-in-place (D4)
  is exactly this; no special handling needed beyond not clobbering other
  accounts.

## Migration Plan

None required. Existing `provider/default` records, scalar `api_key` values, and
accounts created via CLI keep working through existing read-path fallbacks
(`Store.Get` `provider` → `provider/default`; `BuildAccountCredential` scalar
fallback). Rollback = revert the code; nothing written by the new paths is
unreadable by the old code.

## Open Questions

- Should the CLI `auth login` flow prompt for an optional label interactively
  (after showing the resolved identity) rather than only accepting `--account`?
  Deferrable — resolver API supports either.
- Should `providers account rename` exist in the CLI to mirror the dashboard
  Rename? Deferrable — no spec requires it; add if users ask.
- Auto-slot numbering after manual deletes can produce sparse numbering
  (`account-2`, `account-4`). Cosmetic; leave as-is unless it confuses users.
