## ADDED Requirements

### Requirement: Emit one access record per request at completion

The system SHALL emit exactly one structured access record for every HTTP
request, at the moment the response completes — regardless of outcome. This
MUST include requests that fail before reaching the upstream attempt loop
(malformed body, unparseable body, failed authentication, rate limiting,
routing unavailable, unresolved model), which produce no upstream attempt.

#### Scenario: Successful proxied request is logged
- **WHEN** a client sends a valid request that is proxied to an upstream provider and a `2xx` response is returned
- **THEN** the system emits exactly one access record after the response completes

#### Scenario: Pre-proxy authentication failure is logged
- **WHEN** a client sends a request with an invalid or missing API key, so the request is rejected before any upstream attempt
- **THEN** the system still emits exactly one access record for that request, reflecting the rejection status

#### Scenario: Unresolved model is logged
- **WHEN** a client requests a model that does not resolve on the requested dialect surface, so the request is rejected before any upstream attempt
- **THEN** the system still emits exactly one access record for that request

#### Scenario: Non-proxied route is logged
- **WHEN** a client requests a route that is served without proxying (for example `GET /v1/models`)
- **THEN** the system emits exactly one access record for that request

### Requirement: Access record contains transport and correlation fields

Each access record SHALL include the following structured fields: HTTP method,
request path, response status code, response bytes written, end-to-end latency,
the client network address, and a request identifier.

#### Scenario: Fields are populated for a completed request
- **WHEN** a request completes and its access record is emitted
- **THEN** the record contains the request method, path, the status code actually written, the number of response bytes written, the elapsed time from request start to completion, the client network address, and a non-empty request identifier

### Requirement: Captured status and bytes reflect the actual response

The system SHALL capture the response status code and the number of response body
bytes that were actually written to the client, even though those values are not
exposed by the standard library response writer after the response has started.

#### Scenario: Non-default status is captured
- **WHEN** a handler writes a response with status `429`
- **THEN** the access record's status field equals `429`

#### Scenario: Written body size is captured
- **WHEN** a handler writes a response body of `N` bytes
- **THEN** the access record's bytes field equals `N`

### Requirement: Access records correlate to request history by request identifier

The request identifier in an access record SHALL be identical to the request
identifier stored in the corresponding request-history record, so the two can be
joined. The request identifier SHALL honor a caller-supplied request id when one
is provided.

#### Scenario: Access record matches the history record
- **WHEN** a proxied request completes and both an access record and a history record are produced
- **THEN** both records carry the same request identifier value

#### Scenario: Caller-supplied request id is honored
- **WHEN** a client sends a request carrying a request-id header
- **THEN** the access record's request identifier equals that caller-supplied value

### Requirement: Access output format is configurable

The system SHALL emit access records in the format selected by the
`TINYROUTE_LOG_FORMAT` setting, which accepts `text` or `json` and defaults to
`text` when unset.

#### Scenario: JSON format
- **WHEN** `TINYROUTE_LOG_FORMAT` is set to `json`
- **THEN** access records are emitted as JSON objects, one per line

#### Scenario: Text format default
- **WHEN** `TINYROUTE_LOG_FORMAT` is unset
- **THEN** access records are emitted in the structured text format

#### Scenario: Invalid format is rejected at startup
- **WHEN** `TINYROUTE_LOG_FORMAT` is set to any value other than `text` or `json`
- **THEN** the server fails to start with a configuration error naming the setting

### Requirement: Access log level is configurable

The system SHALL emit access records at a level controlled by the
`TINYROUTE_LOG_LEVEL` setting, which accepts `debug`, `info`, `warn`, or `error`
and defaults to `info` when unset. The completion access record is emitted at the
`info` level.

#### Scenario: Default level admits the completion record
- **WHEN** `TINYROUTE_LOG_LEVEL` is unset
- **THEN** the per-request completion access record is emitted

#### Scenario: Invalid level is rejected at startup
- **WHEN** `TINYROUTE_LOG_LEVEL` is set to any value other than `debug`, `info`, `warn`, or `error`
- **THEN** the server fails to start with a configuration error naming the setting

### Requirement: Access records exclude upstream-outcome detail

Access records SHALL NOT include the upstream provider, per-attempt status or
latency, overall outcome, or token usage. Those concerns are owned exclusively by
the request-history store and are reachable by joining on the request identifier.

#### Scenario: Upstream fields are absent
- **WHEN** any access record is emitted
- **THEN** it contains no provider, attempt, outcome, or token-usage fields