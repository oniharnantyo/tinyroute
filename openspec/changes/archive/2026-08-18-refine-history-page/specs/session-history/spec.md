## ADDED Requirements

### Requirement: History is queryable by request ID

The history query layer SHALL support fetching a single record by its request ID, returning the complete record including the request body, translated request body, raw response body, and final response body. When no record exists for the ID, the query SHALL report not-found rather than an error.

#### Scenario: Existing ID returns the full record

- **WHEN** history is queried with the ID of a recorded request
- **THEN** the returned record includes all four captured bodies, attempts, usage, and outcome

#### Scenario: Unknown ID reports not-found

- **WHEN** history is queried with an ID that has no record
- **THEN** the query reports not-found without error

### Requirement: List queries exclude body payloads

The history list query SHALL return record metadata (identity, timestamps, routing, attempts, usage, outcome, latency) without the four body payloads. Body payloads SHALL be retrieved exclusively through the by-ID query, so list cost scales with row count rather than body size.

#### Scenario: List results omit bodies

- **WHEN** history is listed through the query layer
- **THEN** the returned summaries carry no request or response body payloads

#### Scenario: Detail fetch supplies bodies

- **WHEN** a record returned by a list query is fetched by ID
- **THEN** the four body payloads are present on the fetched record
