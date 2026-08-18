## ADDED Requirements

### Requirement: History rows display their true HTTP status

The dashboard history list SHALL display, for each row, the HTTP status the client actually received, derived from the record: the status of the first successful (2xx) attempt when one exists, otherwise the last attempt's status, otherwise a mapping of the outcome category to the status the gateway returned (`no_route`→404, `auth_failed`→401, `rate_limited`→429, `body_too_large`→413, `chain_exhausted`→502, `mid_stream_failure`→502, `ok`→200). The rendered badge SHALL show the derived numeric status, not a hardcoded label. Records with malformed attempt data SHALL still render, using the outcome mapping.

#### Scenario: Successful request shows its real status

- **WHEN** a record's attempts contain a 2xx attempt with status 200
- **THEN** the row's status badge displays `200` with the success variant

#### Scenario: Failed chain shows the final failure status

- **WHEN** a record's attempts are `429` then `502` with no 2xx attempt
- **THEN** the row's status badge displays `502` with the destructive variant

#### Scenario: No-attempt failures map from outcome

- **WHEN** a record has outcome `no_route` and no attempts
- **THEN** the row's status badge displays `404`

#### Scenario: Malformed attempts JSON does not break rendering

- **WHEN** a record's attempts field fails to parse
- **THEN** the row still renders with a status derived from the outcome mapping

### Requirement: History is filterable by provider, date range, key, and session

The history list SHALL offer a provider filter as a dropdown whose options are sourced from live topology providers (plus an "All providers" default), a `from` date picker, a `to` date picker, a key filter, and a session filter. The `to` filter SHALL be inclusive of its entire day. All active filters SHALL be preserved in the URL across Load More activation.

#### Scenario: Provider options come from live state

- **WHEN** the user opens the provider filter
- **THEN** the dropdown lists every configured provider and no free-text entry is required

#### Scenario: Date range is inclusive of the end day

- **WHEN** the user filters with `to` set to a date on which requests exist
- **THEN** records from that entire day are included

#### Scenario: Filters survive Load More

- **WHEN** the user has active provider, date, key, or session filters and activates Load More
- **THEN** the additional rows continue to match every active filter

### Requirement: History pagination uses Load More

The history list SHALL paginate by growing the result window (Load More), not by cursor navigation. Each activation SHALL request the next increment of rows while preserving all filters. The page SHALL indicate the number of rows shown, and the result window SHALL be bounded by a server-side maximum.

#### Scenario: Load More appends within the same filter set

- **WHEN** 80 records match the filter and 50 are shown
- **THEN** activating Load More displays the next increment, ordered most-recent-first, with no duplicate or missing rows

#### Scenario: Load More is absent when the window covers all matches

- **WHEN** the number of matching records does not exceed the current window
- **THEN** no Load More control is rendered

### Requirement: Request detail page exposes captured bodies and attempt chain

The dashboard SHALL serve a per-request detail page at `/dashboard/history/{id}` behind dashboard authentication, showing the record's metadata (status, model, provider, latency, tokens), the full attempt chain (provider, model, status, latency per hop), and four body panes: the client request, the translated provider request, the raw provider response, and the final response delivered to the client. Each pane SHALL display its body size, be collapsed by default, pretty-print JSON bodies, and truncate bodies above a size cap with a visible notice. An unknown request ID SHALL render a not-found state with a link back to the history list.

#### Scenario: Detail page shows the four captured bodies

- **WHEN** the user opens the detail page for a proxied request
- **THEN** the client request, translated provider request, raw provider response, and final response are each present as separate collapsible panes

#### Scenario: Oversized bodies are truncated with a notice

- **WHEN** a stored body exceeds the size cap
- **THEN** the pane renders a truncation notice and a bounded prefix of the body

#### Scenario: Unknown request ID

- **WHEN** the user navigates to `/dashboard/history/{id}` for an ID that does not exist
- **THEN** a not-found state renders with a link back to the history list

#### Scenario: Attempt chain renders per hop

- **WHEN** a record contains multiple attempts
- **THEN** each hop's provider, model, status, and latency are displayed in order
