## Why

The dashboard's request history page is unreliable for its core purpose — auditing proxied traffic. Every recorded request is displayed with a hardcoded "500 Error" badge regardless of actual outcome (a string-mismatch: the handler compares `outcome != "success"` but the store writes `"ok"`), the attempt chain never renders because the handler never populates it, date-range filtering supported by the storage layer is absent from the UI, and the four captured request/response bodies (request, provider request, provider response, final response) are fetched on every page load yet never shown.

## What Changes

- Fix the status display: derive the real HTTP status per row from the winning attempt (2xx status) or map the outcome category to the status actually returned to the client (`no_route`→404, `auth_failed`→401, `rate_limited`→429, `body_too_large`→413, `chain_exhausted`/`mid_stream_failure`→502). Remove the hardcoded `200 OK`/`500 Error` badge text.
- Populate and render the attempt chain (provider/model/status/latency per hop) on both the list rows and the detail view.
- Replace the forward-only cursor "Next Page" link with a **Load More** control that grows the result window (`limit += 50`) and preserves every active filter in the URL. Cursor pagination remains available at the query layer; the dashboard no longer uses it.
- Add filter inputs: provider dropdown sourced from live topology providers (not free text), and `from`/`to` date pickers (end-of-day inclusive on `to`), wired through the handler to the existing `Filter.From`/`Filter.To`. Drop the unused `model` filter field or wire it — decided in design.
- Slim the history list query to metadata columns only (no bodies), and add a `Get(ctx, id)` single-record query returning the full record including the four bodies.
- Add a full detail page at `GET /dashboard/history/{id}` (behind dashboard auth) showing: status, model, provider, latency, tokens, attempt chain, and four collapsible JSON panes — Request (client sent), Provider Request (translated), Provider Response (raw upstream), Final Response (client received) — pretty-printed, collapsed by default with byte counts, and truncated with a notice above a size cap.
- Show timestamps with date + time (`Jan 2 15:04:05`) so paginated rows crossing midnight stay unambiguous.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `management-dashboard`: the history view requirement deepens from "filterable, paginated history" to specific behaviors — truthful per-row status derivation, provider/date/key/session filtering with provider options sourced from live state, Load More pagination preserving filters, and a per-request detail page exposing the four captured bodies behind dashboard auth.
- `session-history`: adds a query-by-request-ID requirement (single-record fetch returning full bodies) so detail views load one record without dragging bodies into list queries.

## Impact

- `internal/dashboard/handler.go` — `handleHistoryView` rewrite (status derivation, filter parsing, limit accumulation) and new `handleHistoryDetailView`; new route registration.
- `internal/dashboard/view_history.templ` — filter form (dropdown, date pickers), status badge, Load More, Inspect → navigate to detail; new `view_history_detail.templ`.
- `internal/history/query.go` — `Querier` gains `Get(ctx, id)`; `Filter` unchanged (already supports provider/key/session/from/to).
- `internal/history/sqlite/query.go` — new `Get` implementation; `List` selects a slim column set (bodies excluded).
- No schema migration needed: all four body columns and the `attempts` JSON already exist and are populated.
- Dashboard detail endpoint sits behind the existing `protectedMux` (password auth) — captured bodies contain full prompts, so no new exposure surface.
