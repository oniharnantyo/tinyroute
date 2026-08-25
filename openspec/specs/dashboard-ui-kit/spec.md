# dashboard-ui-kit Specification

## Purpose
Defines the templui component kit, shadcn theme, and framework-free interaction layer that all dashboard views are composed from.
## Requirements
### Requirement: Views are composed from templui components
All dashboard views (`view_*.templ`) SHALL build their UI from templui component packages (button, badge, card, input, table, selectbox, radio, breadcrumb, alert, skeleton, tabs, separator, dropdown, dialog, sheet, chart) instead of hand-rolled markup with ad-hoc Tailwind class chains for those primitives.

#### Scenario: A view renders a data table
- **WHEN** a view displays tabular data (keys, routes, history, failures, models)
- **THEN** the table is composed from `table.Table/Header/Body/Row/Head/Cell` rather than a hand-rolled `<table>` with utility classes

#### Scenario: A view renders a button
- **WHEN** a view displays a primary, secondary, destructive, outline, or ghost action
- **THEN** it uses `button.Button` with the corresponding typed variant constant (`button.VariantDefault`, `VariantSecondary`, `VariantDestructive`, `VariantOutline`, `VariantGhost`), never raw variant strings

#### Scenario: Variant props are typed constants
- **WHEN** any templui component accepts a variant or size
- **THEN** the value passed is the component package's exported constant, not a string literal

#### Scenario: A view renders a chart
- **WHEN** a view displays a data series over time
- **THEN** it composes the templui `chart` component rather than hand-rolled SVG or a third-party chart script, and the series data reaches the chart through server-rendered `data-*` attributes read from the DOM — script content SHALL NOT interpolate data values directly

### Requirement: Shared wrapper components provide semantic variants
The dashboard SHALL provide wrapper components in `internal/dashboard/components/` — `StatusBadge`, `SearchInput`, `EmptyState`, `KPICard`, `AlertBanner` — that map semantic intent (success, warning, error, info, neutral) onto templui primitives, since templui's badge and alert components ship no such variants.

#### Scenario: Status pill rendering
- **WHEN** a view displays a health, connection, tier, or scope status
- **THEN** it renders `StatusBadge` with a semantic variant, and no view hand-rolls a status pill with inline utility classes

#### Scenario: Empty state rendering
- **WHEN** a list or table has no rows to show
- **THEN** the view renders `EmptyState` (icon + title + message) instead of a bespoke empty-state block

#### Scenario: Flash message rendering
- **WHEN** a handler surfaces a success or error flash message
- **THEN** the view renders `AlertBanner` with the matching semantic variant

### Requirement: Dashboard uses the shadcn preset theme
The dashboard SHALL define the shadcn CSS variables (`--background`, `--foreground`, `--primary`, `--card`, `--border`, `--input`, `--ring`, `--radius`, etc.) for the base-vega/stone preset in `internal/dashboard/assets/input.css`, with dark mode applied via a `dark` class on the root `<html>` element. Base styles SHALL reference theme tokens (`bg-background`, `text-foreground`, `border-border`) rather than hard-coded slate/indigo utilities.

#### Scenario: Component theme resolution
- **WHEN** any templui component renders
- **THEN** its `bg-background`/`text-foreground`-style classes resolve to defined CSS variables, producing correctly readable text and surfaces (no unstyled or white-on-white rendering)

#### Scenario: Dark mode application
- **WHEN** any dashboard page loads
- **THEN** the root `<html>` element carries the `dark` class so the dark variable block applies to the whole dashboard

#### Scenario: Base layer uses tokens
- **WHEN** `input.css`'s base layer styles the body and default borders
- **THEN** it applies `bg-background text-foreground` and `border-border` instead of hard-coded `bg-slate-950 text-slate-100` / `border-slate-800`

### Requirement: Interactions use templui overlays, not Alpine.js
Modals and drawers SHALL be implemented with templui `dialog` and `sheet` components using attribute-based triggers (`dialog.Trigger(ctx)`, `dialog.Close(ctx)`, `dialog.TriggerFor(id)`). The dashboard SHALL NOT load Alpine.js, and no view SHALL contain Alpine directives (`x-show`, `x-model`, `@submit.prevent`, `x-data`).

#### Scenario: Modal interaction
- **WHEN** the user opens the add-provider, model-picker, or confirmation modal
- **THEN** a templui Dialog opens via a trigger-attributed button and closes via `dialog.Close` or its dismiss behavior, with `dialog.js` providing the behavior

#### Scenario: Drawer interaction
- **WHEN** the user opens a history row detail or the manual config panel
- **THEN** a templui Sheet (side panel) opens and closes without any Alpine.js involvement

#### Scenario: Alpine removal
- **WHEN** the migration is complete
- **THEN** no Alpine `<script>` tag is present in any layout and `grep` for Alpine directives across `internal/dashboard/` returns nothing

### Requirement: Inline filtering runs without a framework
Client-side list filtering (provider, client, and model searches; model-picker list filtering) SHALL be provided by a dependency-free JavaScript helper (`assets/filter.js`) driven by data attributes, replacing Alpine.js `x-model`/`x-show` bindings.

#### Scenario: Search filtering
- **WHEN** the user types into a provider, client, or model search field
- **THEN** matching list items remain visible and non-matching items are hidden by `filter.js` without a page reload or any framework dependency

### Requirement: Form posts target existing handler routes
All mutating forms SHALL submit natively (form POST) to the pre-existing handler routes with unchanged field names. Flows that require client-side fetch behavior (plan preview, device-code polling) SHALL use plain JavaScript.

#### Scenario: Native form submission
- **WHEN** the user submits an add-provider, key, settings, or client-configuration form
- **THEN** the browser performs a standard POST to the existing route and the handler behaves exactly as before the migration

### Requirement: Loading and scroll polish effects
Skeleton loading placeholders SHALL animate with the vendored `shimmer` utility. Scrollable containers whose content overflows (history table wrapper, provider-detail model list, overflowing card grids, scrolling sidebar) SHALL apply the vendored `scroll-fade` utility; containers that do not overflow SHALL NOT apply it.

#### Scenario: Shimmering skeleton
- **WHEN** a loading placeholder (skeleton) is rendered
- **THEN** it carries the `shimmer` class and animates while content loads

#### Scenario: Scroll fade only on overflow
- **WHEN** a container's content overflows its scroll area
- **THEN** the container applies `scroll-fade` so the overflowing edge is masked
- **WHEN** a container never overflows in practice
- **THEN** no `scroll-fade` class is applied to it

### Requirement: Existing dashboard behavior is preserved
The migration SHALL NOT change handler routes, form field names, authentication, or any functional behavior specified in the `management-dashboard` capability. Existing dashboard tests SHALL continue to pass; only markup-level assertions may be updated where the markup legitimately changed.

#### Scenario: Test suite after migration
- **WHEN** `go test ./...` runs after the migration
- **THEN** all pre-existing dashboard tests (handler, auth, clients, catalog) pass

#### Scenario: Functional parity
- **WHEN** the user completes any pre-existing flow (login, add provider, connect OAuth, manage models, configure a client, create/revoke keys, change password, view history)
- **THEN** the outcome is identical to before the migration
