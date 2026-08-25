# management-dashboard Specification (delta)

## MODIFIED Requirements

### Requirement: Observe views render live gateway state

The dashboard SHALL render: an overview of request volume, success rate, token usage, and provider health; a providers list; a filterable, paginated request history; and API keys. The overview, providers, and history views SHALL source from existing read APIs (history querier, topology watcher, health store, credential store) and MUST NOT mutate state. The API keys view is a management surface whose mutations are specified in "API keys are managed from the dashboard".

The dashboard SHALL NOT serve a routes view: no sidebar entry for routes SHALL
appear, and `GET /dashboard/routes` SHALL not be a registered route.

#### Scenario: overview reflects current state
- **WHEN** the user opens the overview
- **THEN** request volume, success rate, token totals, provider health, and recent failures are displayed from live state

#### Scenario: history is filterable and paginated
- **WHEN** the user applies filters (provider/key/outcome/time) and paginates
- **THEN** matching history rows are returned through the existing history querier

#### Scenario: routes view is gone
- **WHEN** an authenticated user views the dashboard sidebar
- **THEN** no Routes entry appears
- **WHEN** `GET /dashboard/routes` is requested
- **THEN** the response status is `404`

### Requirement: Combos section in dashboard navigation

The dashboard SHALL provide a Combos section reachable from the sidebar
navigation, positioned between Providers and History, using the Lucide layers
icon. The section SHALL list all configured combos.

#### Scenario: Combos nav entry is reachable

- **WHEN** an authenticated user views the dashboard
- **THEN** the sidebar SHALL contain a Combos entry between Providers and
  History, with the layers icon, linking to the combos list