# provider-credentials Specification

## Purpose
TBD - created by archiving change add-oauth-providers. Update Purpose after archive.
## Requirements
### Requirement: The outbound credential is resolved dynamically per hop

A provider's credential SHALL be a strategy that yields a current token on demand, not a static string baked at config load. At each hop the proxy SHALL resolve the token immediately before building the outbound request and SHALL pass it to the hop dialect for formatting. A static-key credential SHALL return its configured value with no I/O.

#### Scenario: Static credential behaves exactly as before

- **WHEN** a provider declares a static API key (the existing `api_key` / `credential_var` form)
- **THEN** the proxy SHALL send the identical outbound header it sends today
- **AND** SHALL perform no refresh, no file read, and no network call on the request path

#### Scenario: OAuth credential yields a live access token

- **WHEN** a provider declares an OAuth refresh credential with a stored refresh token
- **AND** its access token is unexpired
- **THEN** the proxy SHALL send that access token as the outbound credential
- **AND** SHALL NOT refresh it

#### Scenario: Expired access token is refreshed before the request

- **WHEN** the stored access token is expired or within its refresh lead window
- **THEN** the credential SHALL refresh it before returning
- **AND** the new access token and expiry SHALL be persisted to the credential store

### Requirement: Refresh tokens are stored protected and never logged

The credential custodian SHALL persist refresh tokens (and any `client_secret`) in a file written atomically via temp-file-plus-rename at mode `0600`. Tokens SHALL never be written to logs at any level. The store SHALL be reloadable, and an invalid store SHALL be rejected while the previously loaded set continues to serve.

#### Scenario: Credential file is mode 0600 and atomically written

- **WHEN** a token is stored or refreshed
- **THEN** the credential file SHALL have mode `0600`
- **AND** a crash mid-write SHALL NOT leave a partially-written file

#### Scenario: Tokens are never logged

- **WHEN** a refresh occurs at debug log level
- **THEN** the log entry SHALL NOT contain the refresh token, access token, or `client_secret`

#### Scenario: Invalid store does not disrupt service

- **WHEN** the credential file becomes malformed while the daemon runs
- **THEN** the daemon SHALL continue using the last valid set
- **AND** SHALL log the failure
- **AND** SHALL NOT exit

### Requirement: Plaintext credentials are never exposed in CLI output

No CLI command SHALL print a stored access token, refresh token, or `client_secret` after it has been stored. Listing commands SHALL show at most a masked connectivity indicator and the token's expiry.

#### Scenario: Listing shows a masked indicator only

- **WHEN** `tinyroute provider list` or `tinyroute auth status` is run for a connected provider
- **THEN** the output SHALL indicate that the provider is connected and show the access-token expiry
- **AND** SHALL NOT contain the access token, refresh token, or `client_secret`

### Requirement: Concurrent refresh is deduplicated

When multiple requests require a refresh of the same credential simultaneously, the credential SHALL perform exactly one refresh network call and all waiters SHALL receive its result. A successful refresh SHALL be cached for a short window so requests arriving immediately afterward reuse it without a second call.

#### Scenario: A burst collapses to one refresh

- **WHEN** fifty concurrent requests arrive for a provider whose access token is expired
- **THEN** exactly one refresh network call SHALL be made
- **AND** all fifty requests SHALL proceed with the resulting token

#### Scenario: Result is cached briefly

- **WHEN** a refresh succeeds and another request arrives within the cache window
- **THEN** no second refresh call SHALL be made
- **AND** the cached token SHALL be returned

### Requirement: Refresh failure triggers the provider auth cooldown

A refresh failure, or a 401 from the upstream after a refresh, SHALL be classified as a non-retryable auth failure and SHALL cool the provider down. It SHALL NOT trigger chain failover or a retry storm.

#### Scenario: Refresh failure cools the provider without failover

- **WHEN** a refresh returns an error or the upstream rejects the refreshed token with 401
- **THEN** the provider SHALL be cooled down for the configured auth-failure duration
- **AND** no further hop in the chain SHALL be attempted for that request

### Requirement: Refresh honors per-provider profiles

Different providers SHALL refresh using different bodies and headers, expressed as per-provider refresh profiles rather than per-provider code. A profile SHALL specify at minimum the body format and whether an HTTP Basic header or `client_secret` is included.

#### Scenario: A JSON-body profile with client_id only

- **WHEN** a provider's profile specifies a JSON body without a client secret
- **THEN** the refresh request SHALL post JSON containing `grant_type`, `refresh_token`, and `client_id`
- **AND** SHALL NOT include a `client_secret`

#### Scenario: A profile requiring HTTP Basic auth

- **WHEN** a provider's profile specifies a Basic-auth header
- **THEN** the refresh request SHALL include an `Authorization: Basic` header derived from the client id and secret

### Requirement: `tinyroute auth login` runs the OAuth flow interactively

`tinyroute auth login [provider]` SHALL gather a missing provider by `Select` from the OAuth-capable presets (single candidate auto-selects; non-TTY yields a clear error), execute that provider's OAuth flow, and store the resulting tokens. The device-code flow SHALL print the verification URI and user code and poll until completion.

#### Scenario: Zero args in a TTY prompts for the provider

- **WHEN** `tinyroute auth login` is run in a terminal with no provider argument
- **THEN** the command SHALL offer a `Select` of OAuth-capable presets
- **AND** SHALL proceed with the chosen provider's flow

#### Scenario: Device-code flow polls to completion

- **WHEN** a provider uses the device-code flow
- **THEN** the command SHALL print the verification URI and user code
- **AND** SHALL poll the token endpoint until the user authorizes or the flow expires
- **AND** SHALL store the resulting access and refresh tokens on success

#### Scenario: Non-TTY with no argument yields a clear error

- **WHEN** `tinyroute auth login` is run without a TTY and no provider argument
- **THEN** the command SHALL fail with an error naming the provider and how to supply it

### Requirement: `tinyroute auth import` supports any provider without a flow

`tinyroute auth import [provider]` SHALL accept a refresh token (and the `client_id`, token endpoint, and scopes needed to use it) and store it, enabling providers whose OAuth flow tinyroute does not implement or whose flow is temporarily broken.

#### Scenario: A pasted refresh token is stored and usable

- **WHEN** a refresh token and required constants are provided to `auth import`
- **THEN** they SHALL be written to the credential store
- **AND** a subsequent proxied request SHALL refresh and use the resulting access token

### Requirement: OAuth flow cancellation discards credential updates

When an OAuth login flow (`runPKCEFlow` or `runDeviceCodeFlow`) is cancelled by context signal (such as SIGINT / Ctrl+C), timeout, or network failure, the command SHALL abort immediately, shut down any local HTTP listeners or polling loops, and SHALL NOT persist any token or credential record to the credential store.

#### Scenario: SIGINT during PKCE callback waiting aborts without saving

- **WHEN** an OAuth PKCE authorization is waiting for local HTTP callback
- **AND** a SIGINT (Ctrl+C) signal is received
- **THEN** the local HTTP listener SHALL shut down cleanly
- **AND** the command SHALL return `context.Canceled`
- **AND** no record SHALL be saved to the credential store

#### Scenario: SIGINT during device code polling aborts without saving

- **WHEN** an OAuth device code flow is polling for user authorization
- **AND** a SIGINT (Ctrl+C) signal is received
- **THEN** polling SHALL stop immediately
- **AND** the command SHALL return `context.Canceled`
- **AND** no record SHALL be saved to the credential store

### Requirement: The PKCE authorize request uses the provider's registered redirect URI

The PKCE authorize request SHALL build its `redirect_uri` from the preset's `callback_host` and `callback_path` when declared, and SHALL fall back to `http://127.0.0.1:<port>/callback` when they are absent. The local HTTP callback handler SHALL serve the preset's `callback_path` so the registered URI and the listening path match. A provider-owned preset that requires a specific registered redirect URI (host and/or path) SHALL declare both fields, because OAuth providers compare loopback redirect URIs as exact strings — only the port may vary.

#### Scenario: codex redirects to its registered localhost URI

- **WHEN** `tinyroute auth login codex` builds the authorize request
- **THEN** the `redirect_uri` SHALL be `http://localhost:1455/auth/callback`
- **AND** the local callback handler SHALL accept the request at `/auth/callback`

#### Scenario: A loopback-flexible preset uses the default redirect

- **WHEN** a preset declares no `callback_host` or `callback_path` (such as claude)
- **THEN** the `redirect_uri` SHALL be `http://127.0.0.1:<port>/callback`
- **AND** the callback handler SHALL serve `/callback`

### Requirement: Provider-declared extra authorize parameters are included

The PKCE authorize request SHALL include every key/value pair declared in the preset's `extra_params` as additional query parameters. A preset that declares no `extra_params` SHALL send none.

#### Scenario: codex sends its required extra parameters

- **WHEN** `tinyroute auth login codex` builds the authorize request
- **THEN** the request SHALL include `codex_cli_simplified_flow=true`, `originator=codex_cli_rs`, and `id_token_add_organizations=true`

#### Scenario: iflow sends its required extra parameters

- **WHEN** `tinyroute auth login iflow` builds the authorize request
- **THEN** the request SHALL include `loginMethod=phone` and `type=phone`

#### Scenario: cline identifies by client_type rather than client_id

- **WHEN** `tinyroute auth login cline` builds the authorize request
- **THEN** the request SHALL include `client_type=extension`
- **AND** the token exchange request SHALL also include `client_type=extension`

#### Scenario: A preset without extra params sends none

- **WHEN** a preset declares no `extra_params` (such as claude)
- **THEN** the authorize request SHALL include only the standard OAuth/PKCE parameters

### Requirement: The PKCE authorize request carries the preset's client_id

A provider-owned PKCE preset (one that authenticates against a fixed upstream OAuth client) SHALL declare its `client_id`, and the authorize request SHALL send it. A provider-owned PKCE preset that omits `client_id` is a configuration defect and SHALL fail rather than sending an empty `client_id`.

#### Scenario: antigravity sends its Google client_id

- **WHEN** `tinyroute auth login antigravity` builds the authorize request
- **THEN** the request SHALL include the antigravity Google `client_id`
- **AND** SHALL NOT produce a "Missing required parameter: client_id" error

### Requirement: The PKCE authorize scope is percent-encoded with %20

The PKCE authorize request SHALL percent-encode spaces in its query string as `%20`, not `+`, so strict providers that reject `+` in the `scope` parameter accept the request.

#### Scenario: A multi-scope request encodes spaces as %20

- **WHEN** a preset declares more than one scope (such as codex's `openid profile email offline_access`)
- **THEN** the authorize query SHALL separate scopes with `%20`
- **AND** SHALL NOT contain a `+` in the encoded query

### Requirement: Device-code flows send provider-declared device headers with a stable device id

A device-code preset MAY declare a `device_header_profile`. When declared, the device authorization request, the token poll, and every subsequent refresh request SHALL carry that profile's headers, including a `device_id` that is generated once per connection and reused unchanged across all of those requests. The `device_id` SHALL be persisted with the credential so refresh continues to use the same value. A device-code preset that declares no profile SHALL send no extra device headers.

#### Scenario: kimi sends X-Msh headers with a stable device id

- **WHEN** `tinyroute auth login kimi` performs the device flow
- **THEN** the device authorization request SHALL include the `X-Msh-*` headers
- **AND** the `X-Msh-Device-Id` SHALL be identical on the device request, the token poll, and later refreshes
- **AND** SHALL NOT produce a "Missing user_code parameter" error

#### Scenario: A device preset without a profile sends no device headers

- **WHEN** a device-code preset declares no `device_header_profile` (such as github)
- **THEN** the device requests SHALL send only the standard form parameters

### Requirement: Non-standard flows run dedicated runners, not the generic ones

Providers whose login is not an OAuth authorize-code or RFC 8628 device flow SHALL declare a dedicated `flow_type` and SHALL be dispatched to a provider-specific runner. The generic `runPKCEFlow` and `runDeviceCodeFlow` SHALL NOT be used for them.

#### Scenario: qoder runs its device-token poll flow

- **WHEN** `tinyroute auth login qoder` is invoked
- **THEN** it SHALL run the qoder runner that opens `qoder.com/device/selectAccounts` and polls `openapi.qoder.sh` for a `dt-` token
- **AND** SHALL NOT route through `runDeviceCodeFlow`

#### Scenario: trae runs its login-guidance flow

- **WHEN** `tinyroute auth login trae` is invoked
- **THEN** it SHALL run the trae runner that follows the login-guidance + `ExchangeToken` sequence
- **AND** SHALL NOT route through `runPKCEFlow`

### Requirement: User-registered apps acquire client_id interactively

When a PKCE preset declares no `client_id` (a user-registered OAuth app such as gitlab), `auth login` SHALL interactively prompt for the user's OAuth app `client_id`, and for `client_secret` when the provider's refresh profile requires it, when a TTY is attached. In a non-TTY context it SHALL fail with a clear error naming the missing value. The supplied credentials SHALL be stored with the credential and used for authorize, token exchange, and refresh.

#### Scenario: gitlab prompts for the user's OAuth app client_id

- **WHEN** `tinyroute auth login gitlab` runs in a terminal
- **THEN** it SHALL prompt for the GitLab OAuth app `client_id`
- **AND** the authorize request SHALL send that client_id
- **AND** SHALL NOT produce a "Missing required parameter: client_id" error

#### Scenario: A user-registered app in a non-TTY yields a clear error

- **WHEN** `tinyroute auth login gitlab` runs without a TTY
- **THEN** it SHALL fail with an error naming `client_id` and how to supply it

### Requirement: Auth subcommands accept an account label

`providers auth set`, `providers auth login`, and `providers auth import` SHALL accept
an `--account <name>` flag. When set, the credential SHALL be written under the
`provider/<name>` key (and, for `set`, into `Provider.Accounts[]` for that named
account) instead of the implicit `default`. When unset, behavior SHALL be identical to
today (single-key providers and `"provider/default"` records are unchanged). The flag
SHALL be honored alongside the existing interactive/non-interactive control flags.

#### Scenario: `auth set --account` writes into the named account

- **WHEN** `tinyroute providers auth set openai --account work` is run with a key
- **THEN** the key SHALL be stored as the credential of the `work` account
- **AND** SHALL NOT overwrite `Provider.APIKey`

#### Scenario: `auth login --account` keys the OAuth record by account

- **WHEN** `tinyroute providers auth login codex --account team2` completes the OAuth flow
- **THEN** the resulting tokens SHALL be stored under the `provider/team2` key
- **AND** any existing `default` record SHALL remain untouched

#### Scenario: Omitting `--account` preserves legacy behavior

- **WHEN** `providers auth set` / `login` / `import` is run without `--account`
- **THEN** the credential SHALL be written exactly as before this change
- **AND** existing single-credential providers SHALL behave identically


