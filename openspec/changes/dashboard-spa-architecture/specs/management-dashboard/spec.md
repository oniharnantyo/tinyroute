## MODIFIED Requirements

### Requirement: Observe views render live gateway state

The dashboard SHALL render: a windowed overview of request volume, success rate, token usage, average latency, provider health with traffic, a request-volume chart, and top models by token usage; a providers list; a filterable, paginated request history; and API keys. The overview, providers, and history views SHALL source from read-only state (history aggregates and querier, topology watcher, health store, credential store) and MUST NOT mutate state. The API keys view is a management surface whose mutations are specified in "API keys are managed from the dashboard".

The dashboard SHALL NOT serve a routes view: no sidebar entry for routes SHALL appear, and no routes endpoint SHALL be registered.

The overview SHALL NOT render a recent-failures list; failure investigation is served by the history view's outcome filter.

#### Scenario: overview reflects current state
- **WHEN** the user opens the overview with a window of 1h, 24h, 7d, or 30d (default 24h when absent or unsupported)
- **THEN** request count, success rate, token totals, and average latency are computed over only the records whose timestamps fall within that window, and each renders via the KPI card wrapper with compact number formatting for token counts

#### Scenario: window selection navigates with a query parameter
- **WHEN** the user selects a different window tab on the overview
- **THEN** the active window is reflected in the URL query parameter without a full page reload, so windowed URLs are shareable and restore the chosen window on load

#### Scenario: overview renders a traffic chart
- **WHEN** the overview renders for a window
- **THEN** a request-volume chart renders, bucketed server-side with bucket width derived from the window length, and empty buckets render as zero-height rather than being skipped

#### Scenario: provider panel combines health with window traffic
- **WHEN** the overview renders
- **THEN** each configured provider shows its cooldown status from the health store alongside its windowed request count and success rate, and each provider row links to that provider's detail view

#### Scenario: top models are ranked by windowed token usage
- **WHEN** the overview renders for a window containing records
- **THEN** a top-models table ranks models by combined input and output tokens within the window

#### Scenario: overview auto-refreshes
- **WHEN** the overview remains open in a browser
- **THEN** it refreshes its data periodically without user action so statistics stay current

#### Scenario: overview no longer lists failures
- **WHEN** the overview renders
- **THEN** no failures table is present, and failed requests remain inspectable through the history view's outcome filter

#### Scenario: history is filterable and paginated
- **WHEN** the user applies filters (provider/key/outcome/time) and paginates
- **THEN** matching history rows are returned through the existing history querier

#### Scenario: routes view is gone
- **WHEN** an authenticated user views the dashboard sidebar
- **THEN** no Routes entry appears

### Requirement: Combo cards can be toggled enabled or disabled

Each combo card SHALL carry an enable/disable switch (`role=switch`) in the card footer, left of the edit action, reflecting the combo's current state. Activating the switch SHALL submit a JSON mutation carrying the combo name to the combos toggle API route. The handler SHALL flip the combo's disabled flag, persist configuration, and the SPA SHALL refresh the combos list with feedback. Because the gateway watches the configuration file, the change SHALL take effect on the live gateway without a restart.

#### Scenario: Toggle flips state and persists
- **WHEN** the user activates the switch on enabled combo `coding-priority`
- **THEN** the dashboard SHALL persist `disabled: true` for that combo and re-render the card with the switch off and muted styling

#### Scenario: Toggle takes effect without restart
- **WHEN** a combo is disabled from the dashboard while the gateway is running
- **THEN** the next request for that combo name SHALL fail with the explicit disabled error, with no gateway restart

#### Scenario: Toggle is a plain form POST
- **WHEN** the switch is activated
- **THEN** the mutation is a single JSON POST to the session-protected toggle API route, carried by the shared API client with no bespoke scripting, covered by the dashboard's existing cross-origin protections

### Requirement: Request detail page exposes captured bodies and attempt chain

The dashboard SHALL provide per-request inspection behind dashboard authentication, showing the record's metadata (status, model, provider, latency, tokens), the full attempt chain (provider, model, status, latency per hop), and four body panes: the client request, the translated provider request, the raw provider response, and the final response delivered to the client. The inspector SHALL open from a history row as an in-app drawer that is dismissible (Escape, backdrop, close control) without side effects. Each pane SHALL display its body size, be collapsed by default, pretty-print JSON bodies, and truncate bodies above a size cap with a visible notice. An unknown request ID SHALL show a not-found message with a way back to the history list.

#### Scenario: Detail page shows the four captured bodies
- **WHEN** the user opens inspection for a proxied request
- **THEN** the client request, translated provider request, raw provider response, and final response are each present as separate collapsible panes

#### Scenario: Oversized bodies are truncated with a notice
- **WHEN** a stored body exceeds the size cap
- **THEN** the pane renders a truncation notice and a bounded prefix of the body

#### Scenario: Unknown request ID
- **WHEN** the user opens inspection for an ID that does not exist
- **THEN** a not-found message renders with a way back to the history list

#### Scenario: Attempt chain renders per hop
- **WHEN** a record contains multiple attempts
- **THEN** each hop's provider, model, status, and latency are displayed in order

### Requirement: Dashboard serves the embedded SPA bundle

The dashboard SHALL serve the SPA's hashed static assets under `/dashboard/assets/` from the embedded bundle, with no runtime dependency on external hosts. The legacy component behavior scripts remain served for the transitional templ pages until those pages are retired.

#### Scenario: SPA assets are served
- **WHEN** a logged-in session requests an asset under `/dashboard/assets/`
- **THEN** the response is the embedded file with a 200 status

#### Scenario: SPA shell loads the bundle
- **WHEN** any dashboard route renders the SPA shell
- **THEN** the shell references the hashed asset bundle served by the gateway itself
