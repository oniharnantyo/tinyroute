## ADDED Requirements

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
