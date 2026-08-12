## Context

The dashboard (`internal/dashboard/`) is a server-rendered templ + Alpine.js app.
Each page assembles a `PageData` struct in a handler, renders a `templ` component,
and wraps it in `Layout(title, activeTab, ...)`. Mutations are plain
`<form method=POST>` submissions that rewrite topology via
`ParseRawTopology` → `WriteTopology` and redirect with `?flash=`/`?error=`.

Two "provider" notions exist: `topo.Providers[name]` (added) and `preset.All()`
(41 templates). The CLI lists presets as a **flat** table tagged by AUTH
(oauth/api key/—) and TIER (free/freemium); `Preset.Category` is empty for all.
The OAuth flow lives in `internal/cli/auth.go` (PKCE authorize, device-code
polling, token exchange → `credential.NewOAuthRefreshable`) but is CLI-coupled;
`internal/auth` is only keystore + ratelimiter. A reference Next.js app
(`9router/`) implements browser OAuth as a generic `/api/oauth/[provider]/[action]`
route driving PKCE/device/exchange → store, with per-provider OAuth config in its
registry — tinyroute's `Preset` already carries the equivalent fields.

## Goals / Non-Goals

**Goals:**

- Providers page: all providers, grouped by tier/auth, clickable Title-Cased cards.
- Provider detail: one merged Models section; configure-from-detail for
  unconfigured presets; OAuth Connect for OAuth-capable providers.
- Reuse existing endpoints and data; OAuth flow extracted (not duplicated).

**Non-Goals:**

- No new endpoints beyond OAuth routes; no topology/preset schema changes.
- No category taxonomy (data absent).
- No changes to CLI provider/model/auth commands.

## Decisions

### D1 — Group by tier/auth, not category
Sections **Free Tier → OAuth → API Key**, mirroring CLI signals. Category is empty
across all presets; the CLI never groups by it.

### D2 — Free Tier is a priority pull
A preset with a non-empty `tier` goes into Free Tier **regardless of auth**; each
preset appears in exactly one section.

### D3 — Provider cards are clickable, not button-driven  *(revises earlier CTA choice)*
The whole card is an `<a>` to the detail view. No Manage/Configure buttons, no
base URL, no health flag on the card. Configured-vs-not is communicated by the
detail page (D9), not by per-card CTAs.

### D4 — Title-Cased display names
A `titleCase` helper splits on `-`/`_`/space and capitalizes each word
(`opencode-zen`→`Opencode Zen`). Display name = preset `DisplayName` if present,
else `titleCase(name)`. Applied in both the card and the detail heading.

### D5 — One merged Models section  *(supersedes the earlier two-grid choice)*
Replace the separate Whitelisted + Available grids with a single **Models**
section. The handler builds one annotated list (`CatalogModelItem{ID, Name,
Whitelisted}`): all catalog models (marked whitelisted or not) followed by any
whitelisted custom model absent from the catalog. Whitelisted rows render
Test/Remove; the rest render "+ Add" (POST `/dashboard/models/add`). A filter +
show-more keep large catalogs usable.

### D6 — Detail page serves unconfigured presets  *(supersedes earlier configured-only choice)*
Relax the `!ok → redirect` guard: an unconfigured preset opens the detail page
with an "encourage to configure" message + a Configure action (POST
`/dashboard/providers/add` with `preset_name`) and no model list. Truly unknown
names still redirect ("Provider not found").

### D7 — Remove Endpoint Settings
Drop the read-only advanced-settings section from the detail page. Base URL and
dialect remain visible in the detail header.

### D8 — In-browser OAuth via an extracted runner
Extract the PKCE/device/token logic from `internal/cli/auth.go` into a reusable
`internal/oauth` package: `ConfigFromPreset`, `AuthorizeStart`, `Exchange`,
`DeviceStart`, `DevicePoll`, `Tokens` (shaped for `credential.NewOAuthRefreshable`).
The CLI keeps working; the dashboard imports the new package.

### D9 — Dashboard OAuth routes mirror the reference `[provider]/[action]` pattern
Add `GET /dashboard/providers/{name}/oauth/start` (build authorize URL or request
device code; persist `code_verifier`+`state` server-side), `GET
/dashboard/oauth/callback` (verify state, `Exchange`, store), and device
start/poll endpoints. Flow type is derived from `preset.FlowType` (PKCE default;
`device_code` → device flow).

### D10 — Connect button + status on the detail page
For OAuth-capable providers, the Connections area shows connection status (from
`credential.Store.ListMasked`, already available) and a **Connect** button that
hits the start route. Non-OAuth providers keep the existing API-key paste.

## Risks / Trade-offs

- **`management-dashboard` not yet in main specs** → this change's MODIFIED
  deltas target a capability introduced by the un-archived `serve-dashboard`
  change. Archive `serve-dashboard` first. (Validation already passes on delta
  structure.)
- **OAuth runner extraction correctness** → porting PKCE/device logic out of
  `cli/auth.go` must preserve provider quirks (device header profiles e.g. kimi,
  extra authorize params, refresh profiles). → Mitigation: table-driven tests
  with `httptest.Server` covering PKCE, device poll interval/expiry, and OAuth
  error responses; port logic verbatim, parameterize by preset config.
- **Large catalogs** → flat model grid is heavy for providers like OpenRouter.
  → Mitigation: Alpine filter + show-more (carried over).
- **OAuth state/verifier storage** → must be tamper-proof and per-session.
  → Mitigation: store in the existing session cookie / signed server-side state;
  verify `state` at callback.
- **Security** → tokens are secrets: never logged; the runner and routes must not
  emit plaintext tokens (per `security.md`); credential files stay `0600`.

## Open Questions

- Should `freemium` be visually distinct from `free` within Free Tier? Current
  decision: merged, distinguished only by the `free_note` badge.
- OAuth `state`/`code_verifier` persistence mechanism (session cookie vs. signed
  in-memory store) — to confirm during D9 implementation.
