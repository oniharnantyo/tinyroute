# Tasks — dashboard-spa-architecture

Spec deltas for this change are already written; the implementation work is
the one behavioral gap the deltas introduce (overview window ↔ URL query)
plus verification of the parity claims the specs now make.

## 1. Spec artifacts

- [x] 1.1 Write proposal.md re-chartering dashboard-ui-kit and the management-dashboard deltas
- [x] 1.2 Write delta specs (dashboard-ui-kit, management-dashboard)
- [x] 1.3 Write design.md (in-place re-charter, dual-mounted transition, window-sync decision)
- [x] 1.4 `openspec validate dashboard-spa-architecture --strict` passes

## 2. Overview window URL sync

- [x] 2.1 Read `?window=` on OverviewPage mount (validate against 1h/24h/7d/30d, fall back 24h) and use it as initial state
- [x] 2.2 On window tab change, update the URL query via `history.replaceState` (no reload) and keep the existing fetch-on-change effect
- [x] 2.3 Verify: loading `/dashboard?window=7d` (or SPA equivalent) restores the 7d window; switching tabs updates the address bar; a bad value falls back to 24h

## 3. Parity verification

- [x] 3.1 Grep sweep: no Alpine directives, no `filter.js` reference, no `alert(`/`confirm(` in `web/src` (only doc comments naming what Banner/ConfirmDialog replace)
- [x] 3.2 Grep sweep: no raw palette utilities (`emerald-`, `rose-`, `amber-`, `violet-`, `bg-black/`) in `web/src` — tokens only (matches were the `translate-y` substring)
- [x] 3.3 `go test ./...` green and `pnpm build` green after task 2 changes (dashboard tests 25.2 s ok; bundle 326.09 kB JS / 37.71 kB CSS)

## 4. Closeout

- [x] 4.1 Mark tasks complete, run `openspec status --change dashboard-spa-architecture`, and leave the change ready for archive alongside the follow-ups
