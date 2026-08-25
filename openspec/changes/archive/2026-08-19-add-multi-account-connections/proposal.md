# Add Multi-Account Connections

## Why

Connecting a second OAuth account or adding a second API key for a provider silently
replaces the existing credential. The runtime fully supports multi-account routing
(`Provider.Accounts[]` with selection strategies and per-account cooldowns) and the
credential store is keyed `provider/account`, but every user-facing write path is
single-slot: the dashboard saves OAuth records under `provider/default` and writes
API keys to the scalar `provider.api_key`, and the CLI's `auth login --account` flag
is accepted but never threaded into the flow runners. Users who need multiple
accounts per provider (personal + work, team pools, key rotation staging) cannot
create them except by hand-editing YAML.

## What Changes

- OAuth connect (dashboard and CLI) resolves an account identity for each new
  connection via a deterministic ladder — explicit name, then claims derived from
  the token response (`id_token` email, JWT access-token claims), then an
  auto-generated free slot — instead of always landing on `provider/default`.
- Connecting with an already-existing account name updates that account in place
  (refresh-token rotation); a resolved name that collides with a *different*
  connection takes the next free slot. No write path ever silently replaces a
  different account's credential.
- Dashboard "Add API key" always appends a `Provider.Accounts[]` entry
  (`{name, type: static, api_key}`); the scalar `provider.api_key` is demoted to a
  legacy migration shim the runtime already only reads when no accounts exist.
- Every credential write path that targets an account also upserts the matching
  `Provider.Accounts[]` entry, closing the topology-linkage gap (stored tokens for
  `provider/jane` are actually used by the router).
- Provider detail page gains per-connection **Reconnect** (in-place token rotation)
  and **Rename** actions alongside the existing per-connection delete.
- CLI fixes: `--account` threads through all OAuth flow runners (device code, PKCE,
  qoder, trae); `providers account add --type oauth_refresh` propagates flow errors
  and no longer appends a credential-less account entry on failure.
- Account names are validated before use as store keys (no `/`, bounded charset);
  identity-derived names are sanitized untrusted input (label use only, no
  signature verification).

Non-goals: new selection strategies, per-account quotas UI, or migrating existing
`provider/default` records (existing fallbacks already cover them; all changes are
additive).

## Capabilities

### New Capabilities

- `provider-account-naming`: the account identity resolution ladder and collision
  rules shared by every credential write path — explicit label → token-derived
  identity → auto-slot; in-place update vs. next-free-slot; account-name validation
  and sanitization.

### Modified Capabilities

- `provider-credentials`: the existing "Auth subcommands accept an account label"
  requirement is implemented as specified (today the label is dropped by the flow
  runners) and extended — when no label is given and a connection already exists,
  the flow resolves identity per the naming ladder instead of overwriting
  `provider/default`.
- `provider-account-management`: `providers account add --type oauth_refresh` must
  store tokens under the named account, upsert the `Accounts[]` entry, and surface
  flow failures instead of swallowing them.
- `management-dashboard`: "OAuth providers can be connected from the dashboard" and
  the provider-detail management requirements change — connect resolves an account,
  add-API-key writes `Accounts[]` entries, and per-connection
  reconnect/rename/delete actions are offered.

## Impact

- `internal/credential/` — account-name validation helper; `Save` collision
  semantics unchanged (resolution happens before save).
- `internal/oauth/` — token-response parsing gains `id_token` and raw access-token
  capture; flow results carry a derived-identity hint.
- `internal/cli/auth.go`, `internal/cli/account.go` — account threading through all
  four flow runners; error propagation in `cmdAccountAdd`.
- `internal/dashboard/handler.go`, `internal/dashboard/view_provider_detail.templ` —
  connect/add-key write paths, reconnect/rename handlers, `oauthStateStore` session
  gains an account field; UI uses existing shadcn-templ components and SVG icons.
- `internal/config/topology.go` — account upsert helper; scalar `api_key` documented
  as legacy shim (no removal).
- Security surface: account names become credentials.json keys and appear in
  topology.yaml — emails used as names are mild PII in a non-secret file; token
  claims used for naming are untrusted and sanitized.
