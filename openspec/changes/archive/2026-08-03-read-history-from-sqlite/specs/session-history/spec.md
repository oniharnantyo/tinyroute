## MODIFIED Requirements

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

## ADDED Requirements

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
