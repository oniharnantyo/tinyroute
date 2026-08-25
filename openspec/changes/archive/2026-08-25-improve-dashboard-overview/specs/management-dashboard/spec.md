## MODIFIED Requirements

### Requirement: Observe views render live gateway state

The dashboard SHALL render: a windowed overview of request volume, success rate, token usage, average latency, provider health with traffic, a request-volume chart, and top models by token usage; a providers list; a filterable, paginated request history; and API keys. The overview, providers, and history views SHALL source from read-only state (history aggregates and querier, topology watcher, health store, credential store) and MUST NOT mutate state. The API keys view is a management surface whose mutations are specified in "API keys are managed from the dashboard".

The dashboard SHALL NOT serve a routes view: no sidebar entry for routes SHALL
appear, and `GET /dashboard/routes` SHALL not be a registered route.

The overview SHALL NOT render a recent-failures list; failure investigation is served by the history view's outcome filter.

#### Scenario: overview reflects current state
- **WHEN** the user opens the overview with a window of 1h, 24h, 7d, or 30d (default 24h when absent or unsupported)
- **THEN** request count, success rate, token totals, and average latency are computed over only the records whose timestamps fall within that window, and each renders via the KPICard wrapper with compact number formatting for token counts

#### Scenario: window selection navigates with a query parameter
- **WHEN** the user selects a different window tab on the overview
- **THEN** the view reloads for the chosen window via a query parameter, so windowed URLs are shareable and work without client-side scripting

#### Scenario: overview renders a traffic chart
- **WHEN** the overview renders for a window
- **THEN** a request-volume chart renders via the templui chart component, bucketed server-side with bucket width derived from the window length, and empty buckets render as zero-height rather than being skipped

#### Scenario: provider panel combines health with window traffic
- **WHEN** the overview renders
- **THEN** each configured provider shows its cooldown status from the health store alongside its windowed request count and success rate, and each provider row links to that provider's detail view

#### Scenario: top models are ranked by windowed token usage
- **WHEN** the overview renders for a window containing records
- **THEN** a top-models table ranks models by combined input and output tokens within the window, composed from the templui table components

#### Scenario: overview auto-refreshes
- **WHEN** the overview remains open in a browser
- **THEN** it reloads periodically without user action so statistics stay current

#### Scenario: overview no longer lists failures
- **WHEN** the overview renders
- **THEN** no failures table is present, and failed requests remain inspectable through the history view's outcome filter

#### Scenario: history is filterable and paginated
- **WHEN** the user applies filters (provider/key/outcome/time) and paginates
- **THEN** matching history rows are returned through the existing history querier

#### Scenario: routes view is gone
- **WHEN** an authenticated user views the dashboard sidebar
- **THEN** no Routes entry appears
- **WHEN** `GET /dashboard/routes` is requested
- **THEN** the response status is `404`
