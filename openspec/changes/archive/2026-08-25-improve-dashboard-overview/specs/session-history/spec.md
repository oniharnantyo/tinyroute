## ADDED Requirements

### Requirement: History is aggregable by time window

The system SHALL compute aggregate statistics over history records whose
timestamps fall within a time window: total request count, success count,
input and output token totals, and average latency. It SHALL additionally
group those records by serving provider (request and success counts), by
served model (combined token totals), and into fixed-width time buckets
(request counts per bucket, with bucket width chosen by the caller).
Aggregates SHALL be computed without returning request or response bodies,
and a window containing no records SHALL yield zero-valued aggregates with no
error.

#### Scenario: Window totals count only records inside the window
- **WHEN** aggregates are computed for a window with a known set of seeded records inside and outside it
- **THEN** request count, success count, token totals, and average latency reflect exactly the inside records

#### Scenario: Empty window returns zeros
- **WHEN** aggregates are computed for a window matching no records
- **THEN** counts and totals are zero and no error is returned

#### Scenario: Per-provider grouping splits by serving provider
- **WHEN** records for multiple providers fall inside the window
- **THEN** each provider's entry carries its own request and success counts summing to the window totals

#### Scenario: Per-model grouping ranks by token usage
- **WHEN** records for multiple served models fall inside the window
- **THEN** each model's entry carries its combined input and output token totals

#### Scenario: Time buckets partition the window
- **WHEN** bucketed counts are computed for a window and bucket width
- **THEN** every record inside the window contributes to exactly one bucket, and buckets spanning the window are returned including empty ones

#### Scenario: Aggregates exclude body payloads
- **WHEN** aggregates are computed
- **THEN** no request or response body content is read or returned
