# API Keys Specification

## Purpose

Manages API key lifecycle including secure generation, storage as digests, authentication, scoping, rate limiting, and revocation for controlling access to the gateway service.

## Requirements

### Requirement: Inbound API keys are minted by the CLI and displayed exactly once

`tinyroute keys create` SHALL generate a key consisting of a fixed `tr_live_` prefix followed by at least 32 bytes from a cryptographically secure random source. The plaintext key SHALL be printed once at creation and never retrievable afterwards. The output SHALL include the exact environment variable assignments a client needs to use the gateway.

#### Scenario: Key is generated and shown once

- **WHEN** `tinyroute keys create --name laptop` is run
- **THEN** a key beginning `tr_live_` MUST be printed
- **AND** the output MUST state that it will not be shown again

#### Scenario: Client configuration is printed with the key

- **WHEN** a key is created
- **THEN** the output MUST include the base URL and auth token environment variable assignments required by an Anthropic-dialect client

#### Scenario: Plaintext is unrecoverable afterwards

- **WHEN** any subsequent command lists or inspects keys
- **THEN** the plaintext key MUST NOT be recoverable from stored data or command output

### Requirement: Keys are stored as digests, never as plaintext

`keys.json` SHALL store, for each key, an identifier, a display name, a short non-secret prefix fragment, a `sha256` digest of the key, a creation timestamp, and its scoping fields. The plaintext key MUST NOT be written to disk. Verification SHALL hash the presented credential and compare against stored digests.

#### Scenario: Storage contains no plaintext

- **WHEN** a key has been created
- **THEN** `keys.json` MUST contain its digest and prefix fragment
- **AND** MUST NOT contain the plaintext key

#### Scenario: Listing shows identifying fragments only

- **WHEN** `tinyroute keys list` is run
- **THEN** each key MUST be shown with its identifier, name, and prefix fragment
- **AND** no full key MUST be shown

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

### Requirement: Keys can be scoped to specific surfaces and models

A key MAY declare an allow-list of `surface:model-glob` patterns. When an allow-list is present, a request SHALL be permitted only if its inbound surface and requested model match at least one pattern. When no allow-list is present, the key SHALL permit any configured route.

#### Scenario: Request within scope is permitted

- **WHEN** a key allows `anthropic:claude-*` and a request on `/v1/messages` asks for `claude-sonnet-4-6`
- **THEN** the request MUST be permitted

#### Scenario: Request outside scope is refused

- **WHEN** a key allows only `anthropic:claude-*` and a request arrives on `/v1/chat/completions`
- **THEN** the request MUST be refused as out of scope
- **AND** no provider request MUST be made

#### Scenario: Absent allow-list permits all routes

- **WHEN** a key declares no allow-list
- **THEN** any request matching a configured route MUST be permitted

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
