## Context

The dashboard (`internal/dashboard/`) renders 11 templ views, ~95% hand-rolled Tailwind with inline slate/indigo utilities. The project already depends on `github.com/templui/templui v1.13.0` and vendors 10 component packages in `internal/dashboard/components/` (badge, button, card, dialog, icon, input, table, toast, aspectratio, utils) — but only `Icon` and `toast.Toaster()` are used. Interactivity comes from Alpine.js (CDN): modals (add-provider, model picker, confirmation), drawers (history detail, manual config), and inline list filtering. `components.json` already declares the `base-vega` preset (stone baseColor, cssVariables) and `templui.json`/`.templui.json` configure the templui CLI. CSS builds via `task tailwind` (Tailwind v3.4.19, `input.css` → go:embedded `styles.css`); templ codegen via `task templ`.

Key gap: `input.css` defines **no shadcn CSS variables** — templui components render `bg-background`/`text-foreground` etc. and would be unstyled without them.

Verified templui v1.13.0 facts that shape this design: exact package names (`selectbox`, `radio`, `dropdown` — not select/radio-group/dropdown-menu); no `empty`, `spinner`, or `field` components; `alert` ships only `VariantDefault | VariantDestructive`; `badge` has no success/warning variants; overlay triggers are attribute-based (`dialog.Trigger(ctx)`, `dialog.Close(ctx)`, `dialog.TriggerFor(id)`); toast JS API is exactly `window.tui.toast.add/close/promise`. API reference during implementation: `.claude/skills/shadcn-templ-component/references/` plus vendored source.

## Goals / Non-Goals

**Goals:**
- Every view built from templui components + a small set of shared wrappers — no hand-rolled cards/badges/buttons/inputs/tables.
- One theme system: base-vega CSS variables, dark mode via `class="dark"`.
- One interaction system: templui Dialog/Sheet + a tiny vanilla-JS filter helper; Alpine.js fully removed.
- All handler routes, form field names, and dashboard behavior unchanged; existing tests pass.
- Shimmer on skeleton loading states; scroll-fade on genuinely overflowing containers.

**Non-Goals:**
- No visual regression to the old slate/indigo look (preset shift accepted).
- No handler/route/API changes, no new endpoints, no server-side search endpoints.
- No Tailwind v4 upgrade (stays pinned v3.4.19).
- No rewrite of view data plumbing (same handler data flows into the same templates).

## Decisions

**D1: Theme via CSS variables in `input.css`, dark-only via `class="dark"` on `<html>`.**
The dashboard has no light mode today; applying the `.dark` variable block to `<html>` preserves that while keeping the standard shadcn `:root`/`.dark` structure for a future light mode. Prefer generating variables via the templui CLI preset flow (components.json already says `base-vega`); hand-write the standard stone palette if the CLI flow doesn't fit. The hard-coded `bg-slate-950 text-slate-100` / `border-slate-800` in the `@layer base` block becomes `bg-background text-foreground` / `border-border`.
*Alternative considered:* tuning variables to mimic the old palette — rejected (user chose the preset shift).

**D2: Semantic variants live in wrappers, not forked components.**
`badge`/`alert` lack success/warning/error variants. Rather than forking vendored component packages (upgrade hazard), create five thin `.templ` wrappers in `internal/dashboard/components/`: `StatusBadge`, `SearchInput`, `EmptyState`, `KPICard`, `AlertBanner`. Wrappers map semantic intent → templui variant + `Class` color overrides. `EmptyState` composes from `card` + `Icon` because templui has no `empty` component.

**D3: Alpine.js interactions map as follows.**
- Modals (add-provider, model picker, confirmation) → `dialog.Dialog/Content/Header/Footer` with `Attributes: dialog.Trigger(ctx)` on a Button; `dialog.TriggerFor(id)` where the trigger is far from the dialog root (model-picker case); `dialog.Close(ctx)` for dismiss buttons. `dialog.js` is already wired in the layout.
- Drawers (history detail, manual config) → `sheet` (side variant). Verify after `templui add sheet` whether it needs its own JS bundle tagged in the layout (templui ships none separately, but confirm).
- Radio groups / selects (client-detail strategy pickers) → templui `radio` + `selectbox`.
- Inline client-side filtering (provider/client/model search, model-picker list filter) → new `assets/filter.js`: a data-attribute-driven vanilla helper (~50–100 lines, no framework). Chosen over server-side filtering to keep snappy UX and avoid new routes; chosen over keeping Alpine to hit the zero-Alpine goal.
- Form submits: remove `@submit.prevent`; native form POSTs to existing routes; existing `fetch`-based flows (plan preview, device-code polling) become plain JS.
Finally the Alpine CDN `<script>` tag is removed from `view_layout.templ`.

**D4: Conversion order simple → complex, `view_keys.templ` first as the reference.**
`view_keys` (purest hand-rolled: table + badges + empty state) establishes the Table/Badge/Button conventions; then login → settings → routes → overview → providers → provider-detail → history → clients → client-detail (most complex: model pickers, radios, drawers) last. `view_layout` gets the `dark` class in Phase 0 and its full conversion at the end.

**D5: Component sourcing via templui CLI.**
Add `alert breadcrumb dropdown label radio selectbox separator sheet skeleton tabs` (optional: `pagination`, `popover`, `form`) with the templui CLI using the existing `templui.json` config — components land vendored in `internal/dashboard/components/`, import paths rewritten to this module. No new Go dependencies.

**D6: Polish from already-vendored utilities.**
`shimmer` and `scroll-fade` already exist in `shadcn-tailwind.css` and compile into `styles.css`. Apply `shimmer` to skeletons/loading placeholders; apply `scroll-fade` only to containers that genuinely overflow (history table wrapper, provider-detail model list, card grids, scrolling sidebar). Progressive enhancement — skip containers that never overflow.

## Risks / Trade-offs

- [Theme shift away from slate/indigo] → Accepted by decision; verify no white-on-white after Phase 0 via `task dev` smoke check.
- [templui overlay JS wiring (sheet/dropdown/tabs bundles)] → After `templui add`, check each package for a `.js` file and wire it exactly like `dialog.js` (served from `assets`, tagged in layout).
- [Dialogs breaking form semantics] → Dialog content renders in-place; keep submit buttons inside the original `<form>`; test every POST flow (add provider/key, settings, client config) against existing routes.
- [Vendored shadcn@4 utilities under Tailwind v3] → They compile today; re-verify after markup changes and `task tailwind` rebuild.
- [Test markup assertions] → If tests assert on removed classes/markup, update assertions only — not behavior.
- [Large diff across 11 views] → Ordered conversion (D4) with `templ generate && gofmt -w . && go build ./...` + visual check after each view; each view is independently revertable in git.

## Migration Plan

Phase 0 theme foundation (variables, `dark` class, CLI component adds, CSS rebuild) → Phase 1 wrappers → Phase 2 Alpine replacement mechanics per D3 → Phase 3 view-by-view conversion per D4 (+ D6 polish applied during conversion) → handler/test alignment. Rollback: the work lands on a branch; any converted view can be reverted individually since handler contracts are untouched.

## Implementation & Polish Notes

- **Provider Models UX (view_provider_detail.templ):** Rather than a flat table, models in the provider detail view are presented as categorized, responsive catalog cards ("Whitelisted" vs "Available in Catalog"). This layout supports interactive model health probing, latency display, and one-click whitelist toggles directly on each card.
- **Skeletons & Shimmer in SSR Context:** The tinyroute dashboard renders synchronously via Go and `templ`, so initial loads do not require client-side placeholder skeletons. The `shimmer` utility is verified and preserved in `styles.css` for future asynchronous client fetches.
- **Scroll-Fade & Toast Theming:** `scroll-fade` is applied to all genuinely scrollable overflow areas (including the history audit table and the client detail model picker dialog list). The vendored `toast.Toaster()` container has been updated to use the stone theme tokens (`--card`, `--border`, `--foreground`).

## Open Questions (Resolved)

- Exact sheet/dropdown JS bundle requirements: `sheet` uses `dialog.js` under the hood; `dialog.js` and `filter.js` are embedded via `assets.go` and served via `/dashboard/assets/`.
- CSS variable preset: stone palette configured with Tailwind v3 `hsl(var(--...)/<alpha-value>)` mapping.

