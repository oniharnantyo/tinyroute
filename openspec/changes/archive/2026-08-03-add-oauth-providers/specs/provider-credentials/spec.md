## ADDED Requirements

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
