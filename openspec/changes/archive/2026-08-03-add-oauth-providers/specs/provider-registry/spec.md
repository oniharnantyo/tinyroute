## ADDED Requirements

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

A preset MAY carry OAuth metadata (flow type, client id, authorize/token/device endpoints, scopes, and refresh profile) referenced by name from provider entries. A provider entry SHALL NOT need to repeat these constants; it SHALL reference the preset.

#### Scenario: A provider inherits OAuth constants from its preset

- **WHEN** a provider references an OAuth-capable preset by name
- **THEN** the resolver SHALL supply that preset's client id, endpoints, scopes, and refresh profile to the credential
- **AND** the provider entry SHALL remain terse

#### Scenario: Only OAuth-capable presets are offered for login

- **WHEN** `tinyroute auth login` lists providers
- **THEN** only presets that declare OAuth metadata SHALL appear
- **AND** static-key-only presets SHALL NOT

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
