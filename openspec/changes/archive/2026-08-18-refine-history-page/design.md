## Context

The dashboard history page (`internal/dashboard/handler.go` `handleHistoryView` + `internal/dashboard/view_history.templ`) renders rows from `history.Querier.List`. Three defects motivate this change:

1. **Status display is fiction.** `handler.go:1274-1277` derives `statusCode` by comparing `rec.Outcome != "success"`, but the proxy writes `core.OutcomeOK = "ok"` (`internal/core/types.go:38`). The comparison is always true, so every row gets 500; the templ then renders hardcoded `200 OK`/`500 Error` badge text. The computed `StatusCode` is never displayed.
2. **The attempt chain never renders.** `HistoryRowItem.Attempts` is never populated — `Summary.Attempts` (a JSON string) is dropped in the handler.
3. **Bodies are paid for but never shown.** `sqlite/query.go` `List` fetches `request_body`, `response_body`, `translated_request_body`, `raw_response_body` for every row; the UI displays none of them. Date-range filtering exists in `Filter`/storage but has no UI. The "Next Page" link drops the `session` filter.

All four bodies are already captured by the proxy (`proxy.go:533-536`) and stored — no schema change is needed. Storage already indexes `(provider, timestamp DESC)`.

## Goals / Non-Goals

**Goals:**

- Truthful per-row status derived from attempts/outcome, with the real status code rendered.
- Filter by provider (dropdown from live topology), date range (`from`/`to`), key, session — all preserved across pagination.
- Load More pagination that appends to the result window without losing filters.
- A full detail page per request showing the four captured bodies, attempt chain, and metadata.
- List queries stop fetching body columns (page weight scales with row count, not body size).

**Non-Goals:**

- No schema migration, no new columns, no change to what the proxy records.
- No outcome/model free-text filters beyond what's listed (the dead `FilterModel` field is dropped).
- No bidirectional (Previous) navigation — Load More only.
- No CSV export, no live tailing, no body search.

## Decisions

### D1: Detail is a full page at `GET /dashboard/history/{id}`, not a sheet fragment

Matches the existing `view_client_detail` / `view_provider_detail` precedent and the established `{id}` mux style (`GET /dashboard/providers/{name}/...`). Gives a shareable URL per request and room for four collapsible panes. The Inspect button becomes a link; the existing sheet markup in `view_history.templ` is removed. The route registers on `protectedMux` (password auth) — bodies contain full client prompts, so they must sit behind dashboard auth. Alternative rejected: lazy sheet fragment (no shareable URL, cramped for large bodies).

### D2: Load More via limit accumulation, not cursors

Each activation re-requests `/dashboard/history?provider=…&from=…&to=…&key=…&session=…&limit=N+50`. Because Load More only ever asks for "the first N of the filtered set", a plain `LIMIT N` with the `(provider, timestamp DESC)` index serves it in one seek — no cursor needed, and the filter-dropping bug class disappears (there is no cursor link to build). The query layer's cursor pagination is retained untouched for other consumers (CLI). `limit` is clamped server-side to a max (e.g. 500) to bound page weight. Scroll position on re-render is preserved by appending a `#history-table` fragment identifier to the Load More href. Alternative rejected: mirrored-cursor Prev/Next (more machinery, worse fit for append-only browsing); fetch-and-append fragments (needs JS state handling the otherwise server-rendered page doesn't have).

### D3: Status derivation — winning attempt first, outcome mapping as fallback

The record has no single status column, but `attempts[]` carries per-hop `status`. Displayed status SHALL be:

1. The status of the first attempt with a 2xx status (the winning hop), else
2. The last attempt's status (all hops failed — the client saw the final failure), else
3. For records with no attempts, map `outcome` to the status the proxy actually wrote to the client: `no_route`→404, `auth_failed`→401, `rate_limited`→429, `body_too_large`→413, `chain_exhausted`/`mid_stream_failure`→502, `ok`→200.

The mapping mirrors `proxy.go`'s error writes, so it's faithful by construction. Badge variant classes: 2xx success, 4xx warning, 5xx and 0 (connection failure) destructive. Badge text renders the derived code (`429`, `502`), never a hardcoded string. Malformed `attempts` JSON falls back to the outcome mapping — rendering must never 500 on bad data.

### D4: Filters — provider dropdown from live state, inclusive date range

Provider options come from the topology watcher the handler already depends on (same source as the providers page), prepended with "All providers". This honors the project's options-from-live-state rule; a mistyped free-text provider silently yields an empty result set. `from`/`to` are native `input type="date"` controls; `from` parses as local midnight, `to` as local `23:59:59.999` — storage compares `timestamp <= to.UnixMilli()`, so a bare `to` date would otherwise exclude that day's traffic. Empty strings mean unbounded, matching `Filter`'s zero values. The unused `FilterModel` field and its hidden plumbing are deleted.

### D5: Split reads — slim `List`, new `Get(ctx, id)`

`Querier` gains `Get(ctx context.Context, id string) (Summary, bool, error)`. `sqlite` implements it as a single-row query returning all columns including the four bodies. `List` drops the four body columns from its SELECT — summaries feed table rows only. This makes list cost proportional to row count and detail cost proportional to one record. The `Attempts` JSON string is decoded into `[]core.Attempt` in the handler (for both status derivation and chain rendering) — Summary keeps the raw string field; typed decoding lives at the call site.

### D6: Detail panes — collapsible, pretty-printed, size-guarded

Four panes (Request / Provider Request / Provider Response / Final Response) each show a byte count and a collapsed `<details>` element. Body text passes through `json.Indent` when it parses as JSON (the proxy already normalizes SSE to JSON arrays via `formatResponseAsJSON`); non-JSON bodies render escaped in a `<pre>`. Bodies larger than 512 KB are truncated with a visible notice ("showing first 512 KB of N"). Collapse-by-default keeps a huge streamed transcript from laying out megabytes of DOM on open.

## Risks / Trade-offs

- [Load More re-renders the whole page, losing scroll position] → `#history-table` fragment anchor on the Load More href; if that proves insufficient, a minimal fetch-append enhancement can be layered later without touching the query layer.
- [Large body memory pressure on `Get`] → single record per request, 512 KB display truncation; SQLite streams the row, nothing is retained after render.
- [Malformed `attempts` JSON breaks status derivation or the chain] → decode errors fall back to the outcome mapping (D3) and an empty chain; the page still renders.
- [Date filter timezone confusion] → dates parse in the server's local timezone (same clock that wrote the records); the detail view shows UTC timestamps as stored. Acceptable for a single-host gateway; revisit if multi-timezone deployments appear.
- [`List` no longer returns bodies — hidden dependency for future callers] → spec'd explicitly (session-history delta) so the contract is visible, not incidental.
- [Detail page for an unknown ID] → render a 404-style empty state with a back link, not a bare error.

## Migration Plan

No schema or config migration. Deploy is a restart; existing rows render correctly under the new status derivation immediately (all data needed is already stored). Rollback is reverting the deploy — no data transformation either direction.
