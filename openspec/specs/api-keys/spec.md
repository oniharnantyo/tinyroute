# API Keys Specification

## Purpose

Manages API key lifecycle including secure generation, storage as digests with persisted secrets, authentication, rate limiting, and revocation for controlling access to the gateway service.

## Requirements

### Requirement: API keys are minted by the CLI or the dashboard

`tinyroute keys create` or the dashboard's Create Key action SHALL generate a key consisting of a fixed `tr_live_` prefix followed by at least 32 bytes from a cryptographically secure random source. The plaintext key SHALL be shown exactly once at creation — on CLI stdout, or in the creation dialog with a copy control — and the CLI output SHALL include the exact environment variable assignments a client needs to use the gateway. After creation, the plaintext SHALL remain revealable from the dashboard while the key is active (not disabled); revoked keys MUST NOT be revealable. The plaintext MUST NOT appear in any redirect URL, flash or query parameter, or log.

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

`keys.json` SHALL store, for each key, an identifier, a display name, a short non-secret prefix fragment, a `sha256` digest of the key, the plaintext secret itself (so keys can be re-embedded into downstream client configs and revealed from the dashboard), a creation timestamp, and its optional expiry, rate, and disabled fields. Verification SHALL hash the presented credential and compare against stored digests — never against the plaintext. List output on any surface SHALL show the prefix fragment only, masked, never the full plaintext. The file SHALL be written atomically with mode `0600`.

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

### Requirement: Key creation accepts an optional expiry and rate limit

Key creation — from the CLI (`keys create`) or the dashboard's Create dialog — SHALL accept an optional absolute expiry timestamp and an optional rate limit (requests per interval, the interval a `time.ParseDuration`-valid string). When omitted, the key SHALL have no expiry and no rate limit. Keys created with an expiry SHALL be refused once that timestamp passes, without requiring a change to `keys.json`.

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

### Requirement: Every request is authenticated against a stored key

The service SHALL require a bearer credential on every proxied request and reject requests whose credential is absent, unknown, disabled, or expired. Rejection SHALL use the inbound dialect's native error format. Authentication SHALL occur before route resolution.

#### Scenario: Missing credential is rejected

- **WHEN** a request arrives with no bearer credential
- **THEN** the response MUST be an authentication error in the inbound dialect's native format
- **AND** no provider request MUST be made

#### Scenario: Unknown credential is rejected

- **WHEN** a request presents a credential whose digest matches no stored key
- **THEN** the request MUST be rejected without contacting any provider

#### Scenario: Valid credential proceeds to routing

- **WHEN** a request presents a credential matching an enabled, unexpired key
- **THEN** route resolution MUST proceed

### Requirement: Keys may carry a request rate limit enforced before routing

A key MAY declare a rate limit expressed as a request count per interval, enforced by an in-memory token bucket. Exceeding the limit SHALL reject the request with a rate-limit error in the inbound dialect's native format including a retry hint. A rate-limit rejection is an inbound decision and MUST NOT be treated as a provider failure, MUST NOT trigger failover, and MUST NOT apply any provider cooldown.

#### Scenario: Requests within the limit proceed

- **WHEN** a key limited to 60 requests per minute makes its tenth request in that minute
- **THEN** the request MUST proceed to routing

#### Scenario: Exceeding the limit is rejected with a retry hint

- **WHEN** a key exceeds its configured rate
- **THEN** the response MUST be a rate-limit error in the inbound dialect's native format
- **AND** MUST include a retry hint

#### Scenario: Rate-limit rejection does not affect provider health

- **WHEN** a request is rejected for exceeding a key's rate limit
- **THEN** no chain hop MUST be attempted
- **AND** no provider cooldown MUST be recorded

#### Scenario: Buckets reset on restart

- **WHEN** the daemon restarts
- **THEN** rate-limit buckets MUST begin empty

### Requirement: Revoked keys are hidden from key lists

The dashboard keys view and `tinyroute keys list` SHALL exclude disabled (revoked) keys from their output. `keys.json` SHALL remain the unfiltered source of truth — it is the only recovery path for a revoked key. Revocation SHALL be confirmed before it takes effect and SHALL have no undo path in the CLI or the dashboard; verification SHALL continue to reject revoked keys on every request.

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

### Requirement: Revocation, disabling, and expiry take effect without a restart

`tinyroute keys revoke` SHALL render a key unusable. The daemon SHALL detect changes to `keys.json` by comparing modification time before serving a request and reload it, so that revocation and disabling take effect on the next request. An invalid `keys.json` MUST be rejected while the previously loaded set continues to serve. Expired keys SHALL be refused without requiring a file change.

#### Scenario: Revocation is effective on the next request

- **WHEN** a key is revoked while the daemon is running
- **THEN** the next request presenting that key MUST be rejected

#### Scenario: Invalid key file does not disrupt service

- **WHEN** `keys.json` becomes malformed
- **THEN** the daemon MUST continue authenticating against the last valid set
- **AND** MUST log the failure
- **AND** MUST NOT exit

#### Scenario: Expiry needs no file change

- **WHEN** a key's expiry timestamp has passed
- **THEN** requests presenting it MUST be rejected even though `keys.json` is unchanged

### Requirement: The key file is written only when keys change

`keys.json` MUST be written only in response to a key mutation. Per-request information such as last use time MUST NOT be stored in the key file, because doing so would alter its modification time on every request and defeat the reload-detection mechanism. Last-use information SHALL be derived from the request history instead.

#### Scenario: Serving requests does not modify the key file

- **WHEN** many requests are served without any key being created, modified, or revoked
- **THEN** the modification time of `keys.json` MUST be unchanged

#### Scenario: Last use is derived from history

- **WHEN** `tinyroute keys list` displays last-use information
- **THEN** it MUST be derived from recorded request history rather than from `keys.json`

### Requirement: The key identifier is recorded with every request

Each request record SHALL include the identifier of the key that authorized it, so that usage can be attributed per key. This field SHALL be present from the first record written and MUST NOT be retrofitted, since existing history cannot be corrected.

#### Scenario: Key identifier appears in the record

- **WHEN** a request is served using a given key
- **THEN** the request's history record MUST include that key's identifier

#### Scenario: History can be filtered by key

- **WHEN** history is queried for a specific key identifier
- **THEN** only records authorized by that key MUST be returned

### Requirement: The listener binds to loopback by default

The default listen address SHALL be a loopback address. Because a bound port that spends provider credit is reachable by any local process, authentication SHALL be required by default and MUST NOT be disabled implicitly. Documentation SHALL direct users who need remote exposure to place a reverse proxy in front rather than exposing the service directly.

#### Scenario: Default binding is loopback

- **WHEN** no listen address is configured
- **THEN** the service MUST bind a loopback address

#### Scenario: Authentication cannot be skipped by omission

- **WHEN** no keys exist and a request arrives
- **THEN** the request MUST be rejected
- **AND** the operator MUST be directed to create a key
