## 1. History aggregate data layer

- [x] 1.1 Define `Aggregator` contract and result types in `internal/history/aggregate.go` (`WindowStats`, `ProviderStats`, `ModelStats`, `Bucket`), success predicate documented as `outcome IN ('ok','success')`
- [x] 1.2 Write failing sqlite tests in `internal/history/sqlite/aggregate_test.go`: window totals (inside/outside seeding), empty window → zeros no error, per-provider grouping, per-model grouping with `model_served` fallback to `model_requested`, bucket partitioning including empty-bucket fill, NULL latency excluded from average, legacy `"success"` outcome counted
- [x] 1.3 Implement `internal/history/sqlite/aggregate.go` (`Stats`, `StatsByProvider`, `StatsByModel`, `RequestBuckets`) against the indexed columns only — no body columns; make the tests pass
- [x] 1.4 Verify with `rtk go test ./internal/history/...` and `gofmt -w internal/history/`

## 2. Chart component install

- [x] 2.1 Install the templui `chart` component via the shadcn-templ CLI into `internal/dashboard/components/chart/`; confirm the hashed `components.js` bundle rebuilds and the asset route serves it
- [x] 2.2 Verify the chart's data contract (attribute names / series shape) from the installed component source; record the exact attribute used for series JSON
- [x] 2.3 Regression-check existing components driven by the bundle (dialog open/close, toast via `window.tui`) still work after the bundle change

## 3. Overview handler

- [x] 3.1 Add `HistoryAggregator history.Aggregator` to dashboard `Deps` and wire it in `internal/cli/serve.go`; nil aggregator renders zero-value aggregates without error
- [x] 3.2 Add `windowDurations` map (`1h|24h|7d|30d`, bucket widths 5m/1h/6h/1d) and `?window=` parsing to `handleOverviewView` — missing or invalid value falls back to `24h`; cover with handler tests
- [x] 3.3 Rewrite `handleOverviewView` to assemble `OverviewData` from `Stats`, `StatsByProvider`, `StatsByModel`, and `RequestBuckets` (bucket width from the map), merging topology providers with provider aggregates so idle providers render zero counts; drop the `RecentFailures` population and the `List` scan
- [x] 3.4 Add `formatCompact` helper (1.2k / 2.4M) next to `formatBytes` in `history_helpers.go` with unit tests; verify with `rtk go test ./internal/dashboard/...`

## 4. Overview template

- [x] 4.1 Extend `Layout` with variadic `LayoutOption` and `WithAutoRefresh(d)` emitting `<meta http-equiv="refresh">`; no other call sites change
- [x] 4.2 Rewrite `view_overview.templ`: window tab strip as links (`?window=`) with active state; KPI row via `KPICard` (requests, success rate, tokens compact-formatted with exact values in `title`, average latency); remove the "Gateway Active" badge and the failures table
- [x] 4.3 Add the traffic chart section via the templui `chart` component with the bucket series serialized to the component's `data-*` attribute (no interpolated script content); empty buckets render zero-height
- [x] 4.4 Rebuild the provider panel: cooldown badge via `StatusBadge`, windowed request count and success rate per provider, each row an anchor to `/dashboard/providers/{name}` via `templ.SafeURL`
- [x] 4.5 Add the top-models table via `table.Table/Header/Body/Row/Head/Cell`, ranked by combined window tokens, with the `EmptyState` wrapper when the window has no records
- [x] 4.6 Run `templ generate`, `rtk go test ./...`, and `gofmt -w .`; update `handler_test.go` expectations for the removed failures panel and new sections

## 5. End-to-end verification

- [x] 5.1 Seed the history store with a spread of records (multi-provider, multi-model, successes, failures, legacy `"success"` outcomes), run `go run . serve`, and verify each window tab renders correct KPIs, contiguous chart bars, provider links, and top-models ranking
- [x] 5.2 Confirm the overview auto-refreshes after ~30s and that `?window=7d` survives the refresh
- [x] 5.3 Run `openspec validate improve-dashboard-overview --strict` and resolve any findings
