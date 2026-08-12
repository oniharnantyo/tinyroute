## ADDED Requirements

### Requirement: Dialects emit the correct auth shape for OAuth tokens

A dialect's `AuthHeaders` SHALL distinguish a static API key from an OAuth access token. An OAuth access token SHALL be sent as `Authorization: Bearer <token>`. The inbound caller's credential SHALL never be forwarded upstream; outbound auth SHALL derive solely from the resolved provider credential.

#### Scenario: An OAuth token is sent as Bearer

- **WHEN** a hop resolves an OAuth access token for a provider
- **THEN** the outbound request SHALL carry `Authorization: Bearer <token>`

#### Scenario: The inbound credential is never forwarded

- **WHEN** a request is proxied to an upstream provider
- **THEN** the outbound Authorization header SHALL be set exclusively from the provider's resolved credential
- **AND** SHALL NOT contain the inbound caller's `tr_live_` key

### Requirement: The anthropic dialect falls back from x-api-key to Bearer for OAuth

The anthropic dialect SHALL send `x-api-key` when authenticating with a static API key, and SHALL send `Authorization: Bearer` when authenticating with an OAuth access token. This enables OAuth-subscriber providers (Claude, GitHub Copilot on the anthropic surface, Kimi's anthropic surface) without changing static-key behavior.

#### Scenario: Static key uses x-api-key

- **WHEN** an anthropic-dialect hop authenticates with a static API key
- **THEN** the outbound request SHALL carry `x-api-key: <key>`
- **AND** SHALL NOT carry an OAuth `Authorization: Bearer` header from the credential

#### Scenario: OAuth token uses Bearer

- **WHEN** an anthropic-dialect hop authenticates with an OAuth access token
- **THEN** the outbound request SHALL carry `Authorization: Bearer <token>`
- **AND** SHALL carry the `anthropic-version` header as before
