## Why

The dashboard specs still charter the retired templui implementation while the
shipped dashboard is a Vite + React SPA (`web/`, embedded via `go:embed` and
served under `/dashboard` with a JSON API under `/dashboard/api/*`). During the
2026-08 alignment pass this drift caused real defects: the JSON API layer was a
thinner, divergent reimplementation of the templ handlers (broken client
apply, no-op credential rename/delete, dropped key expiry), and several
spec-mandated behaviors existed only on the templ side. The specs must
describe the architecture that actually ships, so implementation drift becomes
detectable instead of structural.

## What Changes

- **BREAKING** (for the UI kit contract): `dashboard-ui-kit` is re-chartered
  from templui + Alpine-free vanilla JS + `filter.js` + native form posts to
  the React SPA architecture: views composed from the shared React component
  kit (`web/src/components/*`), design tokens locked in `design.md` +
  `web/src/styles/tokens.css`, behavior via React state and `fetch` against
  the JSON API, assets shipped as one embedded hashed Vite bundle.
- The dashboard's data plane is specified as the JSON API surface
  (`/dashboard/api/*`) with the same session auth, loopback Host/Origin
  guards, and secret-handling rules the HTML routes had.
- `management-dashboard` receives narrow behavioral deltas where the SPA
  legitimately changes mechanics, not intent: combo enable/disable submits a
  JSON mutation instead of a form POST; the overview window selector keeps
  windowed state in the URL query (shareable) without a full page reload;
  request inspection renders as an in-app drawer (four captured-body panes)
  rather than a server-routed detail page; the component-behavior-scripts
  requirement becomes an embedded-SPA-assets requirement.
- Records the SPA-era behaviors that are now implemented and were previously
  drifting: client apply preview + explicit confirmation + one-time minted-key
  reveal, model picker dialog with Models/Combos panes, key edit without
  rotation, provider detail JSON with catalog models and lazy
  materialization, combo member account pinning, keys binary status with
  revoked rows excluded.
- The legacy templ HTML routes (`GET /dashboard/{overview,providers,combos,
  history,keys,settings,clients}` and their form POST actions) remain mounted
  during the transition and are explicitly out of scope for removal here; a
  follow-up change retires them.

## Capabilities

### New Capabilities

(none — this change re-charters existing capabilities to match shipped code)

### Modified Capabilities

- `dashboard-ui-kit`: full re-charter — composition (React component kit +
  locked tokens instead of templui primitives), asset delivery (embedded
  Vite bundle under `/dashboard/assets/` instead of component behavior
  scripts), interaction model (React state + JSON `fetch` instead of
  dependency-free `filter.js` and native form POSTs), and the
  behavior-preservation requirement updated to cover the JSON API surface.
- `management-dashboard`: narrow deltas — combo toggle via JSON mutation,
  overview window selection via query-parameter state without page-reload
  wording, request detail as in-app inspector drawer with the four captured
  bodies, component-scripts requirement replaced by embedded SPA assets.

## Impact

- **Specs:** `openspec/specs/dashboard-ui-kit/spec.md` (rewritten),
  `openspec/specs/management-dashboard/spec.md` (deltas above).
- **Code:** `web/src/**` (pages, components, `lib/api.ts`, `types/api.ts`),
  `internal/dashboard/api.go` (JSON handlers), `internal/dashboard/handler.go`
  (route registration, shared `buildProviderDetailData`),
  `internal/dashboard/dist/` (embedded build output), `Taskfile.yml`
  (`build:web` / `dev:web`).
- **Tooling:** pnpm + Vite + Tailwind v4 toolchain in `web/`; Go binary
  embeds `internal/dashboard/dist` via `go:embed all:dist`.
- **Not affected:** gateway proxy paths, auth/keyfile formats, history
  storage, CLI commands. Proxy behavior and security invariants (loopback
  guard, rate-limited login, masked secrets, one-time plaintext reveal)
  carry over unchanged.
