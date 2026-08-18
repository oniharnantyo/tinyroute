## Why

The dashboard in `internal/dashboard/` is ~95% hand-rolled Tailwind (inline `slate-900`/`indigo-600` utilities) even though the project already depends on templui v1.13.0 (the shadcn/ui port for Go templ) and vendors 10 of its components locally. Only `Icon` and `toast.Toaster()` are used; badge, button, card, input, table, and dialog sit unused while the views hand-roll 8+ card variants, 20+ status pills, 50+ buttons, 30+ inputs, 5 full tables, and Alpine.js modals/drawers/filtering. This duplication is a maintenance burden, blocks consistent styling, and leaves two competing interaction systems (Alpine.js CDN + templui's dialog.js).

## What Changes

- **Adopt the templui component library across all 11 views**: replace hand-rolled cards, badges, buttons, inputs, tables, selects, radio groups, and breadcrumbs with templui components (`card`, `badge`, `button`, `input`, `table`, `selectbox`, `radio`, `breadcrumb`, `alert`, `skeleton`, `tabs`, `separator`, `dropdown`).
- **Adopt the shadcn preset theme**: define the CSS variables (`--background`, `--primary`, …) for the `base-vega`/stone preset already declared in `components.json`, apply dark mode via `class="dark"` on `<html>`, and replace hard-coded slate/indigo base styles with theme tokens. The dashboard's visual appearance shifts to the preset — accepted.
- **Create shared wrapper components** in `internal/dashboard/components/`: `StatusBadge`, `SearchInput`, `EmptyState`, `KPICard`, `AlertBanner` — one place for semantic variants templui lacks (badge/alert have no success/warning variants; templui has no `empty` component).
- **Remove Alpine.js entirely**: modals → templui `dialog` (attribute-based triggers), drawers → templui `sheet`, inline client-side filtering → a small vanilla-JS helper (`assets/filter.js`), form submits → native form POSTs to existing handler routes. The Alpine CDN `<script>` tag is dropped from the layout.
- **Apply polish effects**: `shimmer` on skeleton loading states and `scroll-fade` on genuinely overflowing containers — both utilities already vendored in `shadcn-tailwind.css`.
- Handler routes, form field names, and all dashboard behavior stay unchanged; existing tests keep passing.

## Capabilities

### New Capabilities
- `dashboard-ui-kit`: the dashboard's UI implementation layer — templui component adoption, shared wrapper components, preset theming with CSS variables and dark mode, the Dialog/Sheet interaction system (no Alpine.js), and loading/scroll polish effects (shimmer, scroll-fade).

### Modified Capabilities
<!-- None: management-dashboard's behavioral requirements (serving, auth, views, provider/client/key management) are preserved unchanged; this change swaps the UI implementation layer beneath them. -->

## Impact

- **Views**: all 11 `view_*.templ` files in `internal/dashboard/` (plus generated `_templ.go`).
- **Components**: new wrappers in `internal/dashboard/components/`; CLI-added templui component packages (alert, breadcrumb, dropdown, label, radio, selectbox, separator, sheet, skeleton, tabs).
- **Assets**: `input.css` (theme variables), regenerated `styles.css`, new `assets/filter.js`, layout script wiring.
- **Dependencies**: no new Go modules (templui v1.13.0 already in go.mod); Alpine.js CDN dependency removed.
- **Tests**: `handler_test.go`, `auth_test.go`, `clients_test.go`, `catalog_test.go` must keep passing; new rendering tests for wrapper components.
- **Build pipeline**: `task tailwind` (Tailwind v3.4.19) and `templ generate` — no pipeline changes, but styles.css regeneration is required.
