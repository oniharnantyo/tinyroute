## Purpose

Defines request history recording and querying capabilities over SQLite storage.
## Requirements
### Requirement: Records carry the serving account

Each history record SHALL carry the serving `provider/account` in addition to
the serving provider. Replay and session views SHALL surface the account that
served each request so multi-account and combo routing are observable.

#### Scenario: Serving account is recorded for a successful hop

- **WHEN** a request is served successfully by account `a2` of provider `p`
- **THEN** the history record SHALL carry `p/a2` as the serving provider/account

#### Scenario: Serving account is empty when no provider served

- **WHEN** a request fails before any provider/account serves it
- **THEN** the history record's serving provider/account field SHALL be empty

### Requirement: History is queryable by serving account

The history query layer SHALL support filtering by serving `provider/account`
alongside the provider filter, so multi-account traffic can be inspected per
account.

#### Scenario: Filter by account returns only that account's records

- **WHEN** history is queried with a provider/account filter
- **THEN** every returned record SHALL have that provider/account as its serving account

### Requirement: Persist one history record per proxied request

The system SHALL persist exactly one history record for each proxied request,
written after the response to the client has completed. Recording SHALL occur off
the request-critical path so that persisting a record never delays the response
already sent. A record SHALL be produced for every proxied outcome, including
failures where no provider ultimately served the request.

#### Scenario: Successful proxied request produces a record
- **WHEN** a client sends a valid request that is proxied to an upstream provider and the gateway returns a `2xx` response
- **THEN** exactly one history record exists for that request

#### Scenario: Failed request still produces a record
- **WHEN** a proxied request ends without any provider serving it (for example all hops exhausted, no route resolved, or authentication failed downstream of the recorder)
- **THEN** exactly one history record still exists for that request, reflecting the failure outcome

#### Scenario: Recording does not block the client response
- **WHEN** persisting a record is slow
- **THEN** the response to the client has already completed before the record is written

### Requirement: History is queryable by provider and timestamp range with pagination

The system SHALL retrieve history records filtered by serving provider, by a start timestamp, by
an end timestamp, by the authenticating key, by session, or by any combination thereof. Results
SHALL be returned in most-recent-first order and SHALL support cursor-based pagination so that
arbitrarily large result sets can be consumed in bounded pages. The query layer SHALL be the
single read path used by all history views; views SHALL NOT read a separate legacy log file.

#### Scenario: Filter by provider returns only that provider's records
- **WHEN** history is queried with a provider filter
- **THEN** every returned record has that provider as its serving provider

#### Scenario: Filter by timestamp range returns only records within the window
- **WHEN** history is queried with a start and end timestamp
- **THEN** every returned record has a timestamp within the inclusive window

#### Scenario: Filter by key returns only that key's records
- **WHEN** history is queried with a key filter
- **THEN** every returned record was authenticated with that key

#### Scenario: Filter by session returns only that session's records
- **WHEN** history is queried with a session filter
- **THEN** every returned record belongs to that session

#### Scenario: Combined provider and timestamp filter
- **WHEN** history is queried with both a provider and a timestamp range
- **THEN** every returned record matches both the provider and the window

#### Scenario: Pagination returns successive pages and then stops
- **WHEN** history is queried with a page size smaller than the matching result set, and the returned cursor is used for the next query
- **THEN** the union of all pages equals the full matching set with no duplicates and no omissions, and the final query returns no further cursor

#### Scenario: No matches return an empty result
- **WHEN** history is queried with a filter that matches no records
- **THEN** an empty result is returned with no error

### Requirement: Records carry billable usage including cache-creation tokens

The system SHALL record input, output, cache-read, and cache-creation token
counts on each history record when the upstream reports them. Token counts SHALL
default to zero when the upstream does not report usage.

#### Scenario: Cache-creation tokens are recorded
- **WHEN** a successful request's upstream reports cache-creation (cache-write) tokens
- **THEN** the history record's cache-creation token field equals the upstream-reported value

#### Scenario: Usage defaults to zero when unreported
- **WHEN** a request's upstream reports no token usage
- **THEN** the history record's input, output, cache-read, and cache-creation fields are all zero

### Requirement: Records carry end-to-end latency and the serving provider

The system SHALL record, on each history record, the proxy-path latency of the
request and the provider that served it. The serving provider SHALL be empty when
no provider served the request.

#### Scenario: Latency is recorded for a completed request
- **WHEN** a request is proxied and a record is written
- **THEN** the history record carries a latency value reflecting the elapsed proxy-path time for that request

#### Scenario: Serving provider is populated for a successful request
- **WHEN** a request is served successfully by a provider hop
- **THEN** the history record's serving provider field names that provider

#### Scenario: Serving provider is empty when no provider served
- **WHEN** a request fails before any provider serves it
- **THEN** the history record's serving provider field is empty

### Requirement: History records correlate to access records by request identifier

Each history record SHALL carry a request identifier identical to the request
identifier in the access record for the same request, so the history record and
the access record can be joined.

#### Scenario: History identifier matches the access record identifier
- **WHEN** a proxied request completes and both a history record and an access record are produced
- **THEN** both records carry the same request identifier value

### Requirement: History views read from the recorded store
All history read paths — `sessions`, session replay, `log`, and the key last-use derivation —
SHALL read from the same store the recorder writes (the SQLite history database) via the history
query layer. No history view SHALL read a legacy JSONL log that is no longer populated.

#### Scenario: Sessions reflect recorded traffic
- **WHEN** requests have been proxied and recorded, and `tinyroute sessions` is run
- **THEN** the sessions table lists sessions derived from the recorded history (not empty due to a stale file)

#### Scenario: Replay shows a recorded session
- **WHEN** `tinyroute session <id>` is run for a session that exists in recorded history
- **THEN** the replay shows that session's turns

### Requirement: Key identifier is surfaced in history views
The `sessions` view SHALL display the authenticating key identifier for each session, and session
replay SHALL show the key identifier. A session that spans more than one key SHALL display each
distinct key.

#### Scenario: Sessions table includes the key
- **WHEN** `tinyroute sessions` lists a session whose requests were authenticated with key `k_abc12345`
- **THEN** that session's row displays `k_abc12345`

#### Scenario: Replay shows the key per turn
- **WHEN** `tinyroute session <id>` replays a turn authenticated with key `k_abc12345`
- **THEN** the turn output includes `k_abc12345`

### Requirement: Key last-use is derived from recorded history
The key list's last-use timestamp SHALL be derived from recorded history — the most recent record
authenticated with that key — not from a legacy log.

#### Scenario: Last use reflects recent traffic
- **WHEN** a key has authenticated a proxied request that was recorded at time `T`
- **THEN** `tinyroute keys list` shows `T` (or a later recorded use) as that key's last use

#### Scenario: Unused key shows never
- **WHEN** a key has no recorded authentications
- **THEN** `tinyroute keys list` shows its last use as "never"

