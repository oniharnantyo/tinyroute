# provider-registry Specification

## Purpose
TBD - created by archiving change add-oauth-providers. Update Purpose after archive.
## Requirements
### Requirement: A provider declares its credential as a typed block

A provider MAY declare a `credential` block that selects a credential strategy. The existing `api_key` and `credential_var` fields SHALL remain valid as the static-key shorthand and SHALL behave exactly as before when no `credential` block is present. A credential block with an unknown `type` SHALL be a configuration error.

#### Scenario: Static shorthand continues to work unchanged

- **WHEN** a provider declares `api_key` or `credential_var` and no `credential` block
- **THEN** the provider SHALL authenticate with a static API key exactly as before

#### Scenario: An OAuth refresh credential is declared

- **WHEN** a provider declares `credential: { "type": "oauth_refresh", "preset": "codex" }`
- **THEN** the provider SHALL authenticate using the OAuth refresh strategy
- **AND** SHALL source its OAuth constants (client id, endpoints, scopes, refresh profile) from the named preset

#### Scenario: An unknown credential type is rejected

- **WHEN** a provider declares `credential: { "type": "unknown" }`
- **THEN** `ValidateTopology` SHALL return an error naming the provider and the invalid type
- **AND** the daemon SHALL NOT start

### Requirement: Presets carry per-provider OAuth metadata

A preset MAY carry OAuth metadata (flow type, client id, client secret, authorize/token/device endpoints, scopes, refresh profile, callback host, callback path, extra authorize parameters, and device-header profile) referenced by name from provider entries. The callback host, callback path, and extra authorize parameters let a preset express a provider's exact authorize-request shape (for example a fixed registered redirect URI or provider-required query parameters) without provider-specific code in the shared flow. The device-header profile lets a device-code preset declare a header scheme (such as kimi's `X-Msh-*`) without hardcoding it. The flow type MAY be a dedicated value (`pkce`, `device_code`, or a provider-specific runner such as `qoder` or `trae`) so non-standard flows are dispatched to their own runners. A provider entry SHALL NOT need to repeat these constants; it SHALL reference the preset.

#### Scenario: A provider inherits OAuth constants from its preset

- **WHEN** a provider references an OAuth-capable preset by name
- **THEN** the resolver SHALL supply that preset's client id, endpoints, scopes, and refresh profile to the credential
- **AND** the provider entry SHALL remain terse

#### Scenario: Only OAuth-capable presets are offered for login

- **WHEN** `tinyroute auth login` lists providers
- **THEN** only presets that declare OAuth metadata SHALL appear
- **AND** static-key-only presets SHALL NOT

#### Scenario: A preset with a registered redirect URI carries its host and path

- **WHEN** a provider-owned preset (such as codex) requires a specific registered redirect URI
- **THEN** the preset SHALL declare `callback_host` and `callback_path`
- **AND** a loopback-flexible preset (such as claude) MAY omit them

#### Scenario: A preset carries provider-required extra authorize parameters

- **WHEN** a preset's authorize endpoint requires additional fixed query parameters (such as codex's `codex_cli_simplified_flow` or cline's `client_type`)
- **THEN** the preset SHALL declare them as `extra_params`
- **AND** a preset whose authorize endpoint requires none MAY omit `extra_params`

#### Scenario: A device-code preset declares its header profile

- **WHEN** a device-code preset's endpoint requires device-identity headers (such as kimi's `X-Msh-*`)
- **THEN** the preset SHALL declare a `device_header_profile`
- **AND** a device-code preset that needs none MAY omit it

#### Scenario: A non-standard flow declares a dedicated flow type

- **WHEN** a provider's login is not OAuth authorize-code or RFC 8628 device code (such as qoder or trae)
- **THEN** the preset SHALL declare a provider-specific `flow_type`
- **AND** SHALL be dispatched to that provider's runner rather than the generic PKCE/device runners

### Requirement: A preset declares a cost tier surfaced as a tag

A preset MAY declare a `tier` of `"free"` (no credential, no cost), `"freemium"` (a free allocation that still requires a credential), or omit it (standard paid). The tier SHALL be shown as a tag in `provider add` and `provider list` so users can choose providers that do not require spend. A preset MAY carry a short `free_note` describing the free-allocation limits.

#### Scenario: A freemium provider is tagged

- **WHEN** a preset declares `tier: "freemium"`
- **THEN** `provider add` and `provider list` SHALL show a "free tier" tag for that provider
- **AND** SHALL show the `free_note` if present

#### Scenario: A no-auth free provider needs no credential

- **WHEN** a preset declares `tier: "free"` with no credential block and no `api_key`
- **THEN** the provider SHALL be usable without `auth login` or an API key
- **AND** `provider list` SHALL show a "free" tag

#### Scenario: A paid provider shows no free tag

- **WHEN** a preset declares no `tier`
- **THEN** no free/freemium tag SHALL be shown

### Requirement: Provider configuration persistence is atomic and conditional on successful setup

When adding or updating a provider via interactive setup commands (`tinyroute provider add` / `tinyroute add`), configuration changes SHALL NOT be saved to `config.json` before interactive inputs and authentication flows complete, unless the user explicitly opts out of immediate authentication. If an interactive authentication flow is aborted, interrupted (SIGINT/Ctrl+C), or fails, `config.json` SHALL remain unchanged.

#### Scenario: Provider setup with immediate OAuth login saves config only on login success

- **WHEN** a user runs `tinyroute provider add <provider>` for an OAuth-capable provider
- **AND** confirms immediate OAuth login
- **AND** completes the OAuth login successfully
- **THEN** the provider entry SHALL be written to `config.json`
- **AND** the OAuth credential SHALL be saved to the credential store

#### Scenario: Aborted OAuth flow during provider add leaves config.json unchanged

- **WHEN** a user runs `tinyroute provider add <provider>` for an OAuth-capable provider
- **AND** confirms immediate OAuth login
- **AND** interrupts the OAuth flow (via Ctrl+C / SIGINT) or encounters a login failure
- **THEN** `config.json` SHALL NOT be mutated
- **AND** no credential record SHALL be saved

#### Scenario: Declining immediate OAuth login saves unauthenticated provider configuration

- **WHEN** a user runs `tinyroute provider add <provider>` for an OAuth-capable provider
- **AND** explicitly declines immediate OAuth login
- **THEN** the provider entry SHALL be written to `config.json` without credentials
- **AND** instructions to authenticate later SHALL be printed

#### Scenario: Interrupted interactive prompts leave config.json unchanged

- **WHEN** an interactive prompt during `provider add` or `auth set` is interrupted or returns an error
- **THEN** `config.json` SHALL NOT be saved or mutated

### Requirement: A provider may declare multiple named accounts

A provider SHALL support an ordered `accounts` list, where each account is a named
credential block (`static` or `oauth_refresh`), plus a `selection` strategy. The
existing single `api_key` and `credential` fields SHALL remain valid as the
implicit `default` account SHALL behave exactly as before when no `accounts`
are present. A credential block with an unknown `type` SHALL be a
configuration error.

#### Scenario: Static shorthand continues to work unchanged

- **WHEN** a provider declares `api_key` or `credential_var` and no `accounts` block
- **THEN** the provider SHALL authenticate as the `default` account with a static API key exactly as before

#### Scenario: An OAuth refresh credential is declared

- **WHEN** a provider declares `credential: { "type": "oauth_refresh", "preset": "codex" }` and no `accounts`
- **THEN** the provider SHALL authenticate using the OAuth refresh strategy on the `default` account
- **AND** SHALL source its OAuth constants (client id, endpoints, scopes, refresh profile) from the named preset

#### Scenario: An accounts list sets per-account credentials

- **WHEN** a provider declares `accounts: [ { "name": "a", "type": "static" }, { "name": "b", "type": "oauth_refresh" } ]` with `selection: "round_robin"`
- **THEN** each account SHALL carry its own credential strategy
- **AND** the provider SHALL select among them per the strategy

#### Scenario: An unknown credential type is rejected

- **WHEN** any account or credential block declares `type` other than `static` or `oauth_refresh`
- **THEN** `ValidateTopology` SHALL return an error naming the provider and the invalid type
- **AND** the daemon SHALL NOT start

### Requirement: A preset may declare an account capability profile

A preset MAY declare a capability profile used by combo capability reorder
(which members meet `vision`/`pdf`/`audio`/`video` tiers). When present, combo
resolution SHALL use it to tier the panel; when absent, tinyroute SHALL infer
capabilities from member model names.

#### Scenario: Preset capability profile tiers a combo panel

- **WHEN** a preset declares a capability profile and a combo member references that provider/model
- **THEN** the member's capability tier SHALL be taken from the preset profile

#### Scenario: Missing profile is inferred from the model name

- **WHEN** a preset declares no capability profile
- **THEN** the member's capability tier SHALL be inferred from its model name


