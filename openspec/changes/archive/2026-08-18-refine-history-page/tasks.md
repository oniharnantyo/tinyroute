## 1. Storage layer

- [x] 1.1 Add `Get(ctx context.Context, id string) (history.Summary, bool, error)` to the `Querier` interface in `internal/history/query.go`
- [x] 1.2 Implement `Get` in `internal/history/sqlite/query.go` — single-row query returning all columns including the four bodies; not-found returns `(zero, false, nil)`
- [x] 1.3 Slim the `List` SELECT in `internal/history/sqlite/query.go` to metadata columns (drop `request_body`, `response_body`, `translated_request_body`, `raw_response_body`)
- [x] 1.4 Add a `MaxListLimit` clamp (500) to `List` so the Load More window is bounded server-side
- [x] 1.5 Update `internal/history/sqlite/store_test.go`: cover `Get` (existing ID with bodies, unknown ID), assert `List` returns empty body fields, and cover the limit clamp

## 2. Status derivation

- [x] 2.1 Add a status-derivation helper (first 2xx attempt → else last attempt → else outcome map: `no_route`→404, `auth_failed`→401, `rate_limited`→429, `body_too_large`→413, `chain_exhausted`/`mid_stream_failure`→502, `ok`→200) with unit tests covering each branch and malformed attempts JSON
- [x] 2.2 Add an attempts-decoding helper (`Summary.Attempts` JSON string → `[]core.Attempt`) that tolerates malformed input by returning an empty slice; unit-test both paths
- [x] 2.3 Verify badge variant mapping rules: 2xx success, 4xx warning, 5xx and 0 destructive

## 3. History list view

- [x] 3.1 Rewrite `handleHistoryView` filter parsing: provider, `from`/`to` (local-midnight / end-of-day inclusive), key, session, `limit` (default 50, clamped 1–500); drop the dead `FilterModel` plumbing
- [x] 3.2 Populate `HistoryRowItem` with the derived status and decoded attempts; format timestamps as date + time (`Jan 2 15:04:05`)
- [x] 3.3 Replace the provider free-text input with a dropdown sourced from live topology providers plus an "All providers" default
- [x] 3.4 Add `from`/`to` date inputs (`type="date"`) to the filter form; keep key and session inputs
- [x] 3.5 Render the status badge from the derived numeric code (no hardcoded `200 OK`/`500 Error` text)
- [x] 3.6 Replace the cursor "Next Page" link with a Load More link that preserves all filters and grows `limit` by 50, targets `#history-table`, and hides itself when the window covers all matches; show a row count
- [x] 3.7 Regenerate templ (`view_history_templ.go`) and update `handler_test.go` coverage: filters parsed and passed to the querier, inclusive `to` date, limit clamping, Load More href carries all filters

## 4. Detail page

- [x] 4.1 Create `view_history_detail.templ` — back link, status/model/provider/latency/tokens header, attempt chain (provider, model, status, latency per hop), four collapsible body panes with byte counts
- [x] 4.2 Add body-pane rendering: `json.Indent` when parseable, escaped `<pre>` otherwise; truncate above 512 KB with a visible notice showing total size
- [x] 4.3 Add `GET /dashboard/history/{id}` to `protectedMux` and implement `handleHistoryDetailView` — `Get` by ID, not-found renders a 404-style state with a back link
- [x] 4.4 Convert the list Inspect control into a link to the detail page; remove the now-unused sheet markup from `view_history.templ`
- [x] 4.5 Add handler tests: detail route returns the four bodies for an existing ID, not-found state for an unknown ID, and the route is password-protected

## 5. Verification

- [x] 5.1 `gofmt -w .` and `go build ./...`
- [x] 5.2 `go test ./...` green, including updated history/dashboard suites
- [x] 5.3 Manual pass: run `go run . serve`, confirm real statuses (mixed 200/429/502 traffic), provider dropdown options, inclusive date filter, Load More preserving filters, and detail panes on a streamed request
