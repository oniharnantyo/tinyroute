# provider-registry Specification (delta)

## MODIFIED Requirements

### Requirement: A provider may declare multiple named accounts

A provider SHALL support an ordered `accounts` list, where each account is a
named credential block (`static` or `oauth_refresh`), plus a `selection`
strategy. The existing single `api_key` and `credential` fields SHALL remain
valid as the implicit `default` account SHALL behave exactly as before when no
`accounts` are present. A credential block with an unknown `type` SHALL be a
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
