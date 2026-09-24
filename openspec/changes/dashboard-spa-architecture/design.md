## Context

The dashboard ships as a Vite + React SPA (`web/`) embedded into the Go
binary (`internal/dashboard/dist`, `go:embed all:dist`) and served under
`/dashboard` alongside a JSON API (`internal/dashboard/api.go`) and the
transitional templ HTML routes (`internal/dashboard/handler.go`). The 2026-08
alignment pass brought the JSON API to parity with the templ handlers
(client plan/apply/reset rewritten as JSON handlers, credential
rename/delete ported with combo-pin rewrites, provider detail extracted into
`buildProviderDetailData`, keys edit added, lazy materialization on
model-add). The specs, however, still charter the templui implementation.
This change records the shipped architecture; see proposal.md for the drift
defects that motivated it.

## Goals / Non-Goals

**Goals:**

- Make `dashboard-ui-kit` describe the React SPA contract: component kit,
  locked tokens, React-state interactions, JSON API mutations, embedded
  bundle delivery, and the security contract the API routes inherit.
- Record the narrow `management-dashboard` behavioral deltas (toggle
  mutation, shareable window query, inspector drawer, SPA assets).
- Close the one implementable gap the delta introduces: overview window
  state synced to the URL query parameter.

**Non-Goals:**

- Retiring the legacy templ HTML routes and their form handlers — a
  follow-up change removes them once OAuth flows move to the SPA.
- Re-implementing OAuth (PKCE/device-code) as SPA surfaces — separate change.
- Any change to gateway proxy behavior, history storage, or keyfile formats.

## Decisions

### D1. Re-charter in place rather than a new capability

`dashboard-ui-kit` keeps its path and Purpose; its requirements are rewritten
as MODIFIED/REMOVED/ADDED deltas. Alternative: a new `dashboard-spa`
capability plus wholesale REMOVED of the old one. Rejected because the
capability's role (the component/interaction contract for dashboard views)
is unchanged — only the substrate is, and a same-path rewrite keeps the
archive history readable as one lineage.

### D2. Spec the contract, not the stack

The rewritten requirements name the component kit's *roles* (Button, Modal,
ConfirmDialog, Banner, StatusBadge) and the token discipline, but avoid
mandating React/Vite internals beyond what is observable: asset delivery
paths, JSON routes, guards, and rendering behavior. A future substrate swap
(e.g. another renderer) would still force a delta, but only at the
asset-delivery and interaction requirements, not across every view rule.

### D3. Dual-mounted routes during transition

The JSON API and the templ HTML routes stay mounted side by side; both sit
behind the same `authMiddleware` + `HostGuardMiddleware` chain, so the
security posture is uniform and the legacy OAuth callback (which redirects
to a templ detail page) keeps working. Removal is deferred to the OAuth-SPA
change so this change carries no auth-flow risk.

### D4. Overview window via URL query, client-side

The SPA reads `?window=` on mount and updates it with
`history.replaceState` on tab change — no page reload, shareable URLs,
restores on load. Alternative: full navigation (`pushState` + refetch).
Rejected: replaceState plus the existing fetch-on-change effect achieves
the spec's shareability without remounting the view.

## Risks / Trade-offs

- [Two sources of truth for handler logic (api.go JSON vs handler.go form
  handlers) invite fresh drift] → Shared builders (`buildProviderDetailData`,
  `saveAccountGesture`, `resolveExistingKey`) are the single homes for
  merged logic; the specs' parity requirement makes divergence a spec
  violation, and route retirement (follow-up) removes the duplicate surface.
- [`design.md`/`tokens.css` discipline can erode like the templ theme did]
  → The locked system is enforced by the Hallmark stamp + gates recorded in
  `design.md`; token-purity is a spec requirement ("Dashboard uses the locked
  design system"), testable by grep for raw palette utilities.
- [Drawer inspector vs routed detail page loses deep-linkable request URLs]
  → Accepted trade-off recorded in the delta; the drawer keeps
  Escape/backdrop dismissal and a not-found path. If deep links become
  needed, add `/dashboard/history/{id}` SPA routing without touching the
  API.

## Migration Plan

1. Land the spec deltas (this change) — no runtime impact.
2. Implement the one code task: overview window ↔ URL query sync.
3. Follow-up change A: OAuth flows as SPA surfaces (JSON start/poll +
   SPA callback page), then retire the templ detail dependency.
4. Follow-up change B: remove legacy templ HTML routes and form handlers;
  delete the shadcn-templ component bundle serving once nothing references it.

Rollback: revert the spec deltas; the window-sync task is independently
revertible with no data impact.

## Open Questions

- None — the deferred items are captured as follow-up changes with their own
  planning cycles.
