## MODIFIED Requirements

### Requirement: Views are composed from the shared React component kit
All dashboard views (`web/src/pages/*.tsx`) SHALL build their UI from the shared React component kit in `web/src/components/` — Button, Input, Select, Modal, ConfirmDialog, Banner, StatusBadge, KpiCard, CodeSnippet, VolumeChart, ProviderLogo, ModelPickerDialog — instead of hand-rolled markup with ad-hoc Tailwind class chains for those primitives. Views MUST use the typed `variant`/`size` props those components expose rather than string class overrides.

#### Scenario: A view renders a data table
- **WHEN** a view displays tabular data (keys, history, models)
- **THEN** the table reuses the page-local table styling conventions rather than introducing a new table style

#### Scenario: A view renders a button
- **WHEN** a view displays a primary, secondary, destructive, outline, or ghost action
- **THEN** it renders the kit's Button with the corresponding variant, never a bespoke button with inline classes

#### Scenario: Variant props are typed constants
- **WHEN** any kit component accepts a variant or size
- **THEN** the value passed is the component's typed prop, not a class string

#### Scenario: A view renders a chart
- **WHEN** a view displays a data series over time
- **THEN** it composes the kit's chart component (VolumeChart) rather than hand-rolled SVG, with series data reaching it through props

### Requirement: Shared wrapper components provide semantic variants
The kit SHALL provide semantic wrapper components that map intent onto the design tokens: `StatusBadge` (success, warning, error, neutral), `KpiCard`, `Banner` (dismissible success/error/warning feedback), `ConfirmDialog` (destructive confirmation), and empty-state blocks.

#### Scenario: Status pill rendering
- **WHEN** a view displays a health, connection, tier, or key status
- **THEN** it renders `StatusBadge` with a semantic variant, and no view hand-rolls a status pill with inline utility classes

#### Scenario: Empty state rendering
- **WHEN** a list or table has no rows to show after loading
- **THEN** the view renders an icon + title + message empty block instead of a bespoke empty-state construction

#### Scenario: Flash message rendering
- **WHEN** a mutation succeeds or fails
- **THEN** the view surfaces feedback through `Banner` (or a minted-key reveal block), never `alert()`/`confirm()`

### Requirement: Dashboard uses the locked design system
The dashboard SHALL define its shadcn-style CSS variables (`--background`, `--foreground`, `--primary`, `--card`, `--border`, `--input`, `--ring`, `--radius`, …) in `web/src/styles/tokens.css`, mapped into Tailwind v4 via `@theme` in `web/src/styles/index.css`, with the system locked by `design.md` at the repo root (genre modern-minimal, one accent, mono-for-machine-data). Base styles SHALL reference theme utilities (`bg-background`, `text-foreground`, `border-border`) rather than hard-coded palette utilities; raw OKLCH/hex values live only in the token block.

#### Scenario: Component theme resolution
- **WHEN** any kit component renders
- **THEN** its `bg-background`/`text-foreground`-style classes resolve to the CSS variables, producing readable text and surfaces in the locked dark theme

#### Scenario: Dark mode application
- **WHEN** any dashboard view loads
- **THEN** the locked dark variable set applies to the whole dashboard — the SPA ships a single dark theme

#### Scenario: Base layer uses tokens
- **WHEN** the base layer styles the body and default borders
- **THEN** it applies the theme utilities instead of hard-coded palette utilities

### Requirement: Interactions run through React state and the JSON API
Modals, drawers, filtering, wizards, and pickers SHALL be React state driven, talking to the gateway exclusively through the JSON API client (`web/src/lib/api.ts`). The dashboard MUST NOT load Alpine.js, htmx, or any runtime DOM-behavior framework; list filtering happens in component state without page reloads.

#### Scenario: Modal interaction
- **WHEN** the user opens the create-key, combo-wizard, or model-picker dialog
- **THEN** it opens and closes via React state with Escape/backdrop dismissal, and no separate behavior script is fetched

#### Scenario: Drawer interaction
- **WHEN** the user opens a history row's inspection drawer
- **THEN** it opens and closes via React state with Escape and backdrop dismissal, with no framework beyond React involved

#### Scenario: Alpine removal
- **WHEN** the SPA bundle is loaded
- **THEN** no Alpine (or other DOM-behavior framework) script or directive is present anywhere in `web/src`

### Requirement: Mutations target the JSON API with the dashboard guards
All mutating actions SHALL POST JSON to `/dashboard/api/*` routes through the API client, with `credentials: 'same-origin'`. These routes sit behind the same session auth and loopback Host/Origin guard middleware as the legacy HTML routes; a 401 from any API route SHALL return the user to the login screen.

#### Scenario: Native form submission
- **WHEN** the user submits a create, edit, delete, toggle, plan, apply, or reset action
- **THEN** the SPA sends a single JSON POST to the corresponding API route via the shared API client and updates from the JSON response, surfacing errors through `Banner`

#### Scenario: Unauthorized API access
- **WHEN** an API route responds 401
- **THEN** the SPA redirects the user to the login view

### Requirement: Loading and scroll polish effects
Loading placeholders SHALL render a visible pulse while a view's data is loading (never an empty state or blank canvas for in-flight data). Scrollable containers with bounded height (history drawer panes, model lists, wizard member lists) SHALL remain fully scrollable, and no container SHALL apply a scroll mask where no overflow exists.

#### Scenario: Shimmering skeleton
- **WHEN** a loading placeholder is rendered
- **THEN** it carries a pulse animation while content loads

#### Scenario: Scroll fade only on overflow
- **WHEN** a container's content overflows its bounded height
- **THEN** the container remains fully scrollable with no clipped content
- **WHEN** a container never overflows in practice
- **THEN** no scroll affordance is rendered

### Requirement: Existing dashboard behavior is preserved by the JSON API
The SPA migration SHALL NOT change functional behavior specified in the `management-dashboard` capability: handler semantics, authentication, form field meaning, and secret-handling rules carry over to the JSON API routes. Existing Go dashboard tests SHALL continue to pass; only test assertions tied to removed templ markup may be updated.

#### Scenario: Test suite after migration
- **WHEN** `go test ./...` runs
- **THEN** the dashboard tests pass against the JSON API routes

#### Scenario: Functional parity
- **WHEN** the user completes any flow (login, add provider, manage models, configure a client, create/edit/revoke keys, change password, view history)
- **THEN** the outcome is identical to the templ-era behavior the specs describe

## REMOVED Requirements

### Requirement: Inline filtering runs without a framework
**Reason**: The dependency-free `filter.js` helper existed to replace Alpine bindings in server-rendered markup. The React SPA performs filtering in component state; a separate behavior script is meaningless.
**Migration**: Filtering behavior is now covered by "Interactions run through React state and the JSON API"; `assets/filter.js` is no longer loaded.

## ADDED Requirements

### Requirement: SPA is built, embedded, and served by the gateway
The dashboard SPA SHALL be built by the standard web toolchain (Vite) into `internal/dashboard/dist` and embedded into the Go binary. The gateway SHALL serve the bundle's hashed assets under `/dashboard/assets/` and the SPA shell for dashboard navigation, with no runtime dependency on external hosts. All dashboard traffic remains excluded from proxy request history.

#### Scenario: Assets are served from the binary
- **WHEN** a logged-in session requests a hashed asset under `/dashboard/assets/`
- **THEN** the response is the embedded file with a 200 status

#### Scenario: SPA shell serves dashboard routes
- **WHEN** an authenticated user navigates to `/dashboard` or a deep dashboard route
- **THEN** the SPA shell is served and renders the corresponding view client-side

### Requirement: JSON API observes the dashboard security contract
Every `/dashboard/api/*` route SHALL sit behind session auth and the loopback Host/Origin guard; login SHALL be rate-limited per client address; sessions SHALL be cookie-based with HttpOnly. Secret material SHALL be masked in list responses, write-only in forms, and any plaintext key (minted client key) SHALL appear exactly once in the response that created it — never in logs, URLs, or error copy.

#### Scenario: Guarded API routes
- **WHEN** an unauthenticated request hits any `/dashboard/api/*` route other than the auth endpoints
- **THEN** the response is 401

#### Scenario: Secrets stay masked
- **WHEN** provider connections or keys are listed
- **THEN** tokens render masked and no plaintext credential appears except the one-time minted-key reveal
