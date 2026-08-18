# API Keys Delta

## ADDED Requirements

### Requirement: Key creation accepts an optional expiry and rate limit

Key creation — from the CLI (`keys create`) or the dashboard's Create dialog —
SHALL accept an optional absolute expiry timestamp and an optional rate limit
(requests per interval, the interval a `time.ParseDuration`-valid string).
When omitted, the key SHALL have no expiry and no rate limit. Keys created
with an expiry SHALL be refused once that timestamp passes, without requiring
a change to `keys.json`.

#### Scenario: CLI creates a key with expiry and rate

- **WHEN** `tinyroute keys create --name ci-bot --expires 168h --rate 60/1m` is run
- **THEN** the created key SHALL carry the computed absolute expiry and the rate limit
- **AND** `tinyroute keys list` SHALL display both

#### Scenario: Omitted options create an unrestricted key

- **WHEN** a key is created with no expiry and no rate options
- **THEN** the stored key SHALL have no expiry and no rate limit

#### Scenario: Dashboard-created key carries the same options

- **WHEN** a key is created from the dashboard Create dialog with a 30-day expiry
- **THEN** the stored key SHALL carry the computed absolute expiry, identical to a CLI-created key

### Requirement: Revoked keys are hidden from key lists

The dashboard keys view and `tinyroute keys list` SHALL exclude disabled
(revoked) keys from their output. `keys.json` SHALL remain the unfiltered
source of truth — it is the only recovery path for a revoked key. Revocation
SHALL be confirmed before it takes effect and SHALL have no undo path in the
CLI or the dashboard; verification SHALL continue to reject revoked keys on
every request.

#### Scenario: Dashboard list excludes revoked keys

- **WHEN** the keys view renders and `keys.json` contains a disabled key
- **THEN** that key MUST NOT appear in the table

#### Scenario: CLI list excludes revoked keys

- **WHEN** `tinyroute keys list` runs against a keystore containing a disabled key
- **THEN** the disabled key MUST NOT be listed

#### Scenario: keys.json keeps revoked records

- **WHEN** a key has been revoked
- **THEN** its record (including digest and secret) MUST remain in `keys.json`

#### Scenario: Revoked key is refused on the request path

- **WHEN** a revoked key's credential is presented after revocation
- **THEN** the request MUST be rejected on the next request without a service restart

## MODIFIED Requirements

### Requirement: API keys are minted by the CLI or the dashboard

`tinyroute keys create` or the dashboard's Create Key action SHALL generate a
key consisting of a fixed `tr_live_` prefix followed by at least 32 bytes from
a cryptographically secure random source. The plaintext key SHALL be shown
exactly once at creation — on CLI stdout, or in the creation dialog with a
copy control — and the CLI output SHALL include the exact environment variable
assignments a client needs to use the gateway. After creation, the plaintext
SHALL remain revealable from the dashboard while the key is active (not
disabled); revoked keys MUST NOT be revealable. The plaintext MUST NOT appear
in any redirect URL, flash or query parameter, or log.

#### Scenario: Key is generated and shown once

- **WHEN** `tinyroute keys create --name laptop` is run
- **THEN** a key beginning `tr_live_` MUST be printed
- **AND** the output MUST state that it will not be shown again in the terminal

#### Scenario: Client configuration is printed with the key

- **WHEN** a key is created via the CLI
- **THEN** the output MUST include the base URL and auth token environment variable assignments required by an Anthropic-dialect client

#### Scenario: Active key is revealable from the dashboard

- **WHEN** the dashboard reveals an active key
- **THEN** the plaintext is unmasked with a copy control
- **AND** it MUST NOT appear in any URL, flash parameter, or log

#### Scenario: Revoked key is not revealable

- **WHEN** a key has been revoked
- **THEN** no dashboard surface SHALL display or copy its plaintext

### Requirement: Keys are stored as digests with a persisted plaintext secret

`keys.json` SHALL store, for each key, an identifier, a display name, a short
non-secret prefix fragment, a `sha256` digest of the key, the plaintext secret
itself (so keys can be re-embedded into downstream client configs and
revealed from the dashboard), a creation timestamp, and its optional expiry,
rate, and disabled fields. Verification SHALL hash the presented credential
and compare against stored digests — never against the plaintext. List output
on any surface SHALL show the prefix fragment only, masked, never the full
plaintext. The file SHALL be written atomically with mode `0600`.

#### Scenario: Storage contains digest and secret

- **WHEN** a key has been created
- **THEN** `keys.json` MUST contain its digest, prefix fragment, and plaintext secret
- **AND** the file MUST be readable only by its owner (mode `0600`)

#### Scenario: Listing shows identifying fragments only

- **WHEN** `tinyroute keys list` is run
- **THEN** each key MUST be shown with its identifier, name, and prefix fragment
- **AND** no full key MUST be shown

#### Scenario: Verification compares digests

- **WHEN** a credential is presented to the gateway
- **THEN** it is verified by comparing its `sha256` digest against stored digests

## REMOVED Requirements

### Requirement: Keys can be scoped to specific surfaces and models

**Reason**: Scope patterns (`surface:model-glob`) added a per-key authorization
concept that neither management surface could set, serving only the client
installer's dialect pinning. For a single-admin gateway the complexity is not
justified; the key model is reduced to mint, see, limit, kill.
**Migration**: `Key.Allow`, `matchesScope`, and `matchGlob` are deleted and
`Verify` loses its `surface`/`model` parameters. Existing `allow` entries in
`keys.json` are ignored on read (those keys become full-access) and are
dropped from the file on its next mutation. Users who relied on dialect
pinning to contain an exposed client credential should rotate that key.

## RENAMED Requirements

- FROM: `### Requirement: Inbound API keys are minted by the CLI and displayed exactly once`
- TO: `### Requirement: API keys are minted by the CLI or the dashboard`
- FROM: `### Requirement: Keys are stored as digests, never as plaintext`
- TO: `### Requirement: Keys are stored as digests with a persisted plaintext secret`
