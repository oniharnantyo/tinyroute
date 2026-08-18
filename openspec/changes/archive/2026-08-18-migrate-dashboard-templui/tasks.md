## 1. Theme foundation

- [x] 1.1 Define base-vega/stone shadcn CSS variables (`:root` + `.dark`) in `internal/dashboard/assets/input.css` — via templui CLI preset if it fits, otherwise hand-written stone palette
- [x] 1.2 Replace hard-coded `bg-slate-950 text-slate-100` / `border-slate-800` base-layer styles in `input.css` with `bg-background text-foreground` / `border-border`
- [x] 1.3 Add `class="dark"` to `<html>` in `view_layout.templ` (and `view_login.templ` if standalone)
- [x] 1.4 Add upstream components via templui CLI: `alert breadcrumb dropdown label radio selectbox separator sheet skeleton tabs` (verify each package's exact JS bundle needs and wire them like `dialog.js` if present)
- [x] 1.5 Rebuild and smoke-check: `templ generate`, `task tailwind` one-shot build, `go build ./...`, verify via `task dev` that variables resolve (no unstyled components)

## 2. Shared wrapper components

- [x] 2.1 Create `StatusBadge` (`components/status_badge.templ`) wrapping `badge.Badge` with success/warning/error/info/neutral variants
- [x] 2.2 Create `SearchInput` (`components/search_input.templ`) composing `input.Input` + `Icon`
- [x] 2.3 Create `EmptyState` (`components/empty_state.templ`) composing `card.Card` + `Icon` (templui has no `empty` component)
- [x] 2.4 Create `KPICard` (`components/kpi_card.templ`) wrapping `card.Card/Header/Content`
- [x] 2.5 Create `AlertBanner` (`components/alert_banner.templ`) wrapping `alert.Alert` with semantic variants via `Class` overrides
- [x] 2.6 Add rendering tests for all five wrappers and run `templ generate && gofmt -w . && go test ./internal/dashboard/...`

## 3. Alpine.js replacement mechanics

- [x] 3.1 Create `assets/filter.js` — data-attribute-driven vanilla filtering helper for provider/client/model searches and the model-picker list
- [x] 3.2 Verify Dialog trigger patterns (`dialog.Trigger(ctx)`, `dialog.Close(ctx)`, `dialog.TriggerFor(id)`) work with existing forms (submit buttons stay inside their `<form>`)
- [x] 3.3 Verify Sheet side-panel behavior for drawers; confirm JS bundle wiring from task 1.4
- [x] 3.4 Convert fetch-based flows (plan preview, device-code polling) to plain JavaScript if they relied on Alpine helpers

## 4. View conversion (simple → complex)

- [x] 4.1 Convert `view_keys.templ` — table + StatusBadge + EmptyState (reference conversion; establishes conventions)
- [x] 4.2 Convert `view_login.templ` — card form + Input + Button + AlertBanner
- [x] 4.3 Convert `view_settings.templ` — settings card + password form
- [x] 4.4 Convert `view_routes.templ` — table + hop badges
- [x] 4.5 Convert `view_overview.templ` — KPICard row + provider health list + failures table
- [x] 4.6 Convert `view_providers.templ` — provider cards + SearchInput + filter.js + add-provider Dialog
- [x] 4.7 Convert `view_provider_detail.templ` — breadcrumbs + connection cards + model table + forms
- [x] 4.8 Convert `view_history.templ` — filter form + table + Sheet detail drawer
- [x] 4.9 Convert `view_clients.templ` — client cards + SearchInput + filter.js
- [x] 4.10 Convert `view_client_detail.templ` — model-picker Dialogs (`TriggerFor`), radio + selectbox pickers, confirmation Dialog, manual-config Sheet
- [x] 4.11 Convert `view_layout.templ` — sidebar/nav on theme tokens, script wiring, remove Alpine CDN `<script>` tag

## 5. Polish

- [x] 5.1 Apply `shimmer` class to all skeleton/loading placeholders
- [x] 5.2 Apply `scroll-fade` to genuinely overflowing containers only (history table wrapper, provider-detail model list, overflowing grids, scrolling sidebar)
- [x] 5.3 Sweep views for leftover hard-coded theme utilities (`slate-`, `indigo-`, `emerald-`-chained pills) — remove or intentionally keep with justification

## 6. Verification

- [x] 6.1 `templ generate && gofmt -w . && go build ./...` clean
- [x] 6.2 `go test ./...` — all pre-existing dashboard tests pass (update markup assertions only where markup legitimately changed)
- [x] 6.3 Manual check via `task dev`: every view renders themed; login flow, all modals/drawers, search filters, toasts, and every form POST work
- [x] 6.4 `grep -r "alpine\|x-show\|x-model\|x-data\|@submit.prevent" internal/dashboard/` returns nothing
