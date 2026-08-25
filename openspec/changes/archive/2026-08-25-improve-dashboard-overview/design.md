## Context

See proposal.md — Why. The current overview computes "statistics" by scanning
one page of history rows (`handler.go:221`, `Filter{Limit: 100}`); the query
layer (`internal/history/query.go`) offers only `Get`/`List`/`LastUseByKey`.
The SQLite store already has the indexes aggregation needs:
`idx_requests_ts (timestamp DESC)` and `idx_requests_provider (provider,
timestamp DESC)` (`internal/history/sqlite/db.go:143`).

Two legacy facts shape the design:

- Outcome values are `core.Outcome*` constants (`"ok"`, `"mid_stream_failure"`,
  …) but older records may carry the legacy string `"success"`
  (`history_helpers.go:61` handles both). Any success predicate must accept
  `'ok'` and `'success'`.
- templ `<script>` content renders literally — dynamic data must flow through
  `data-*` attributes read via `dataset`, never interpolated script bodies.

## Goals / Non-Goals

**Goals:**

- Aggregate reads on the history store that are correct over any window size
  without paging rows into Go.
- Overview handler assembled from a small set of cheap aggregate calls.
- Chart data path that obeys the `data-*` constraint and adds the chart
  component without destabilizing the JS bundle.

**Non-Goals:** p95 latency, cost estimation, all-time totals, client-side
fetch-poll refresh architecture, new history-view filters.

## Decisions

### D1: New `Aggregator` interface, separate from `Querier`

`internal/history/aggregate.go` defines a new contract; the SQLite store
implements it in `internal/history/sqlite/aggregate.go`. The dashboard `Deps`
gains `HistoryAggregator history.Aggregator` (nil-tolerant, like
`HistoryQuerier`).

- Why not extend `Querier`: every existing implementor and test fake would
  break for a concern only the overview needs today; the coding-style rules
  keep contracts in their own files. A separate optional dependency keeps the
  handler honest (nil → zero-value aggregates, page still renders).
- Alternative rejected: computing aggregates in the handler from `List` pages —
  retains the page-cap and multiplies queries.

```go
type Aggregator interface {
    Stats(ctx context.Context, from, to time.Time) (WindowStats, error)
    StatsByProvider(ctx context.Context, from, to time.Time) ([]ProviderStats, error)
    StatsByModel(ctx context.Context, from, to time.Time) ([]ModelStats, error)
    RequestBuckets(ctx context.Context, from, to time.Time, bucketMs int64) ([]Bucket, error)
}
```

`WindowStats`: TotalRequests, SuccessRequests, InputTokens, OutputTokens,
AvgLatencyMs. Success predicate in SQL:
`SUM(CASE WHEN outcome IN ('ok','success') THEN 1 ELSE 0 END)`.

Model grouping keys on `COALESCE(NULLIF(model_served,''), model_requested)`
(failed requests never set `model_served`).

### D2: SQL aggregation, body columns untouched

All aggregate queries select only `COUNT`/`SUM`/`AVG`/`GROUP BY` over the
indexed columns; `requests` body columns are never referenced. Bucket query:
`GROUP BY (timestamp - :from) / :bucketMs` over the window; Go fills empty
buckets between 0 and `(to-from)/bucket` so the chart axis is contiguous.
Providers with zero window traffic are absent from `GROUP BY` results — the
view merges topology providers with the aggregate map so configured-but-idle
providers still render with zero counts.

### D3: Window selection via query parameter, bucket width derived

`?window=1h|24h|7d|30d`, default and invalid-fallback `24h` (spec scenario).
Fixed map `windowDurations` in the handler. Bucket widths target 12–30 bars:
1h→5m, 24h→1h, 7d→6h, 30d→1d. Tabs render as links (shareable URLs, no JS
dependency) styled as a tab strip; the active tab is the current `window`
value. The templui `tabs` component was considered and rejected — it is a
client-side toggle, not navigation, and would break shareable/refreshable
windowed URLs.

### D4: Chart via templui chart component, data through attributes

Install `chart` with the shadcn-templ CLI into
`internal/dashboard/components/chart/`; the hashed `components.js` bundle and
asset embed update accordingly. The series (bucket timestamps + counts) is
serialized to JSON in a `data-chart-series` attribute on the chart container;
the component's script reads it via `dataset` — no interpolated script
content. Known CLI friction to watch: dev-path pinning in the asset handler
and bundle prop-type mismatches.

### D5: Auto-refresh via meta refresh, Layout variadic

`Layout` grows `opts ...LayoutOption`; overview passes `WithAutoRefresh(30s)`
which emits `<meta http-equiv="refresh" content="30">` in the head. Variadic
keeps the other seven call sites untouched. Alternative (JS
`setInterval`+`fetch`) rejected for v1: full-document refresh keeps one
rendering path and the URL/window state for free; the KPI page is cheap to
re-render.

### D6: Failures panel removed, links reuse existing routes

`OverviewData` drops `RecentFailures`; the hardcoded-500 loop dies with it.
Provider rows link via the existing pattern
`/dashboard/providers/{name}` (`templ.SafeURL`, as `view_providers.templ:195`).
The "Gateway Active" pulsing badge is replaced by a neutral count summary
(e.g. providers healthy/cooling) — no fake liveness.

### D7: Compact number formatting helper

`formatCompact(int64) string` (1.2k / 2.4M) in `history_helpers.go` beside
`formatBytes`, used for token and request KPI values; exact values ride along
in `title` attributes.

## Risks / Trade-offs

- [Chart CLI install disturbs the JS bundle] → verify the hashed bundle serves
  and existing dialogs still function after `add chart`; keep the install as
  an isolated commit so a revert is one commit.
- [Chart component script assumes a data shape] → confirm the component's
  expected attribute contract before wiring; if it expects per-series
  elements, serialize one attribute per series.
- [AVG(latency_ms) ignores NULLs by SQL semantics] → desired: failed requests
  without latency excluded from the average; verify with a seeded test.
- [Legacy `"success"` outcomes drift from `core.OutcomeOK`] → predicate
  accepts both; single SQL constant, documented in `aggregate.go`.
- [30d windows scan more rows under GROUP BY] → acceptable at personal-gateway
  scale; timestamp index bounds the scan. If it ever matters, a
  pre-aggregated rollup table is the escape hatch (out of scope).

## Migration Plan

Additive at the data layer (new interface + queries; no schema change, no
migration). Handler and template swap in place. Rollback = revert the
overview handler/template to `List`-based rendering; aggregates can ship
independently since nothing else consumes them yet.
