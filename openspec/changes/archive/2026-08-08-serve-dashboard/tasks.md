# Tasks

## 1. Build pipeline & tooling

- [x] 1.1 Add `templ` toolchain and a `templ generate` step to the build (verify `*.templ` → `_templ.go` codegen)
- [x] 1.2 Add the Tailwind standalone binary (no node) and a CSS build step; emit a single compiled stylesheet
- [x] 1.3 Set up `go:embed` for the compiled CSS and provider logo SVGs so the binary stays self-contained
- [x] 1.4 Vendor the templ UI v2 components used (sidebar, card, table, badge, sheet/dialog, input, button, select, icon) into `internal/dashboard/components/`
- [x] 1.5 Update build scripts/CI to run `templ generate` + `tailwindcss` before `go build`; confirm `go build ./...` still succeeds

## 2. Package skeleton & serving

- [x] 2.1 Create `internal/dashboard/` package with route registration that mounts under `/dashboard/*` on the shared `serve` mux
- [x] 2.2 Confirm dashboard handlers do NOT route through `proxy.Handler` (verify no `/dashboard/*` traffic is written to the history store)
- [x] 2.3 Add `Dashboard` fields to `config.Service` (`Enable`, `Listen` reuse, `PasswordPath`) and the `--no-dashboard` flag; register new vars in the known-`TINYROUTE_` map
- [x] 2.4 Wire conditional dashboard mount + graceful shutdown into `internal/cli/serve.go`, gated by `--no-dashboard`

## 3. Browser auto-open

- [x] 3.1 Detect interactive display (TTY) presence at `serve` startup
- [x] 3.2 Open the dashboard URL via `open` (darwin) / `xdg-open` (linux) / `start` (windows) when interactive; otherwise log the URL

## 4. Authentication

- [x] 4.1 Implement the bcrypt password store for `~/.tinyroute/dashboard.json` (read/write, mode `0600`, atomic tmp+rename)
- [x] 4.2 On first run with no auth file, seed a bcrypt hash of the default password `123456`
- [x] 4.3 Implement login handler: `bcrypt.CompareHashAndPassword`, set `SameSite=Strict` session cookie on success; deny + no cookie on failure
- [x] 4.4 Gate all `/dashboard/*` routes on a valid session cookie (login redirect otherwise)
- [x] 4.5 Implement logout (clear session cookie)

## 5. Request hardening

- [x] 5.1 Reject mutating POSTs whose `Host` header is not a loopback host (`localhost`/`127.0.0.1`)
- [x] 5.2 Throttle login attempts via `auth.RateLimiter` keyed on the client address
- [x] 5.3 Add a no-op CSRF token or rely on SameSite+Host guard per design (confirm coverage)

## 6. Password change

- [x] 6.1 Settings → change-password screen (current, new, confirm) using templ UI form components
- [x] 6.2 POST handler writes the new bcrypt hash atomically; verify new password works and old fails without restart

## 7. Observe views (read-only)

- [x] 7.1 Overview page: KPI cards (requests, success %, token totals), provider-health list, recent-failures table — sourced from `history.Querier`, `topologyWatcher`, `HealthStore`
- [x] 7.2 Providers list page (name, dialect, base URL, model count, health badge, last-used) from topology + health
- [x] 7.3 Routes page (From → Match → Chain hops) from topology
- [x] 7.4 History page: filterable (provider/key/outcome/time) + paginated via `history.Querier.List`; row detail drawer with attempts + bodies
- [x] 7.5 Keys page: list from `keys.json` (masked), scopes, rate spec, last-used via `LastUseByKey`

## 8. Provider management (write)

- [x] 8.1 Extend `preset.Preset` with optional `DisplayName`, `Logo`, `Category` fields (backward-compatible) for card rendering
- [x] 8.2 Add provider brand logo SVGs (`go:embed`) with a generated monogram fallback for missing logos
- [x] 8.3 Add-provider card picker (Dialog): branded preset cards + search/filter + a Custom card
- [x] 8.4 Provider setup view: Connections (via `credential.Store.ListMasked`, filtered by provider) + Connect action (OAuth device/callback or API-key paste)
- [x] 8.5 Provider edit/remove via `ParseRawTopology` → `WriteTopology`; verify `${VAR}` references preserved and atomic `0600` write
- [x] 8.6 Secret fields write-only in UI and masked in any JSON response (no plaintext leakage)

## 9. Model whitelist cards (write)

- [x] 9.1 Lift the probe logic out of `cmdProviderModelTest` into a shared reusable `probe` helper (CLI + dashboard share it)
- [x] 9.2 Model cards: tinyroute name (`provider:model`) + copy control, provider model name, Test action (probe → status+latency), remove control
- [x] 9.3 Add models from catalog (`LoadOrRefreshCatalog`) / live (`FetchProviderModels`) into the whitelist via the mutators

## 10. Icons

- [x] 10.1 Render all icons via templ UI `icon` (Lucide); sidebar nav, status indicators, and actions
- [x] 10.2 Verify no emoji glyphs are emitted anywhere in the rendered UI

## 11. Tests (target ≥80%, per `.claude/rules/testing.md`)

- [x] 11.1 Unit tests: password store (seed/hash/verify/atomic), session cookie, Host/Origin guard, login throttle
- [x] 11.2 Unit tests: dashboard route registration excludes history recording; `--no-dashboard` disables routes
- [x] 11.3 Integration tests: provider add/edit/remove through mutators preserves `${VAR}`; secrets masked in JSON
- [x] 11.4 Integration tests: model test probe, whitelist add/remove, copy-name output
- [x] 11.5 Coverage check across `internal/dashboard/` meets the 80% bar

## 12. Provider list → detail refactor

- [x] 12.1 Vendor templ UI components: `templui apply --preset b2DKLWUNe` (install `templui` CLI if absent); migrate views to the real templ UI `icon` (Lucide), replacing `internal/dashboard/components/icons.go`
- [x] 12.2 Simplify `view_providers.templ` to compact cards: logo (or monogram fallback) + display name + connection count only; render `pre.Logo`/`pre.DisplayName` in the Add dialog
- [x] 12.3 Create `view_provider_detail.templ` (HEADER, CONNECTIONS, MODELS, ADVANCED) and `GET /dashboard/providers/{name}` + `handleProviderDetailView` on the protected mux
- [x] 12.4 Connection data: count helper (`len(Accounts)`, else 1 if direct `APIKey`); detail rows from `Provider.Accounts` + `credential.Store.ListMasked()` (masked token/expiry)
- [x] 12.5 Logos: render `pre.Logo`/provider SVG with monogram fallback for presets lacking one
- [x] 12.6 Model picker: replace free-text add-model with catalog (`LoadOrRefreshCatalog`) / live (`FetchProviderModels`) control; rename section to **Models**
- [x] 12.7 Tests: detail handler (found/not-found), connection-count helper, logo/monogram fallback; keep coverage ≥80%
