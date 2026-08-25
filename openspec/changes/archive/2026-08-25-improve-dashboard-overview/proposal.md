## Why

The dashboard overview simulates monitoring instead of delivering it. "Total
Requests" is actually the count of the last history page (≤100 rows), the
success-rate denominator is a page size, every failure renders as a hardcoded
"500", statistics carry no time window, and nothing on the page links anywhere.
The operator questions a gateway overview exists to answer — how much traffic,
did it succeed, how fast, is anything cooling down, what am I using most — are
either misanswered or absent. The root cause is upstream of the template: the
history `Querier` interface exposes only row retrieval, so the handler
improvised "stats" by scanning one page of rows.

## What Changes

- **Windowed statistics**: the overview scopes all KPIs to a selectable time
  window — 1h / 24h / 7d / 30d tabs, default 24h — driven by real aggregate
  queries, not page scans.
- **New history aggregates**: the SQLite history store grows window-scoped
  aggregate queries (totals with success count and average latency, per-provider
  grouping, per-model grouping, time-bucket grouping for the chart), exposed
  through an extended `Querier`.
- **Traffic chart**: a request-volume-over-time chart on the overview, rendered
  by the shadcn-templ chart component from server-computed buckets, with data
  passed via `data-*` attributes.
- **Provider panel with traffic**: the provider list combines cooldown health
  with windowed request count and success rate, and each row links to the
  provider detail view.
- **Top models table**: models ranked by windowed token usage.
- **Auto-refresh**: the overview reloads every 30 seconds.
- **Removal**: the Recent Failures panel is removed from the overview. Failure
  investigation remains covered by the history view's outcome filter. The
  hardcoded "500" failure badges and the 100-row failure blind spot die with it.
- **Number formatting**: token counts and request counts render compactly
  (e.g. 2.4M), not as raw integers.

Out of scope: p95 latency, cost estimation (no pricing data exists in the
codebase), all-time totals.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `management-dashboard`: the overview requirement changes — statistics become
  window-scoped with selectable tabs; the traffic chart, top-models table,
  provider traffic enrichment, and auto-refresh are added; the recent-failures
  clause is removed (failure investigation stays with the history view).
- `session-history`: new requirement — history is aggregable by time window
  (totals, per-provider, per-model, time buckets), giving the dashboard (and
  any future consumer) a contract for aggregate reads distinct from row
  pagination.
- `dashboard-ui-kit`: the templui component set grows `chart`, with a scenario
  constraining how chart data is passed (server-rendered `data-*` attributes,
  not interpolated script content).

## Impact

- `internal/history/query.go` — `Querier` interface gains aggregate methods.
- `internal/history/sqlite/` — new aggregate queries (`stats.go`), leveraging
  existing `idx_requests_ts` and `idx_requests_provider` indexes.
- `internal/dashboard/handler.go` — `handleOverviewView` rewritten to source
  from aggregates; accepts a `window` query parameter.
- `internal/dashboard/view_overview.templ` — new layout: window tabs, chart,
  provider panel with traffic, top models; failures table removed.
- `internal/dashboard/components/chart/` — new templui chart component
  installed via shadcn-templ CLI; JS bundle (`components.js`) and asset
  wiring updated accordingly.
- Spec deltas as listed above; no CLI, proxy, or config changes.
