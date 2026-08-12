## Why

tinyroute is managed entirely through the CLI and hand-editing `config.json`. For day-to-day operations — watching live traffic, checking provider health, adding a provider or whitelisting a model, rotating keys — a local web dashboard is faster and more discoverable, and it surfaces runtime state (request history, cooldowns, token spend) that the CLI only shows on demand. The dashboard opens automatically on `serve`, giving a zero-friction local control panel with no separate process or port to manage.

## What Changes

- **Web dashboard served by `serve`** on the existing gateway listener (`127.0.0.1:8787`), mounted under `/dashboard/*`. Default ON; opt out with a new `--no-dashboard` flag. Auto-opens in the user's browser only when an interactive display is present (never in headless/container runs).
- **Dashboard auth**: a single password (default `123456`), **bcrypt-hashed** and persisted to `~/.tinyroute/dashboard.json` (mode `0600`, atomic write); changeable from the UI. Login establishes a `SameSite=Strict` session cookie; every mutating POST is gated by a `Host`/`Origin` header check (CSRF / DNS-rebinding defense); login attempts are throttled via the existing `auth.RateLimiter`.
  - **Accepted risk:** the default password stays active until manually changed (no forced first-login change). Mitigated by loopback-only bind, login throttle, and the CSRF/origin guard.
- **Observe views**: KPI/health-first overview (requests, success %, token spend, provider health, recent failures), providers table, routes, request history (filterable + paginated), and keys — all read from existing APIs.
- **Manage actions**: add/edit/remove providers (logo card picker → setup view with connections + model cards), edit routes, manage model whitelists, rotate keys — all routed through existing mutators (`ParseRawTopology` → `WriteTopology`).
- **Model cards** show only: tinyroute model name (`provider:model`, copyable), provider model name, a Test action (reuses the model probe), and a remove button.
- **Toolchain & UI**: templ + Tailwind **standalone binary** (node-free), assets embedded via `go:embed`. Icons are **Lucide via templ UI's `icon` component — never emoji**. New `internal/dashboard/` package.

## Capabilities

### New Capabilities
- `management-dashboard`: a loopback-served, password-gated web UI for observing gateway state and managing topology/keys, opened automatically by `serve` and opt-out via `--no-dashboard`.

### Modified Capabilities
<!-- None. The dashboard consumes existing capabilities as-is: provider-registry, provider-credentials
     (ListMasked), provider-model-management (probe/whitelist mutators), session-history (Querier.List),
     core-routing, and api-keys. No requirement-level changes to those specs. -->

## Impact

- **New code**: `internal/dashboard/` package (HTTP handlers, templ components, auth/session store, embedded CSS/HTML/assets); lifecycle wiring in `internal/cli/serve.go` (dashboard routes on the shared mux, browser-open, graceful shutdown); new fields + `--no-dashboard` flag in `internal/config/service.go` (and the known-`TINYROUTE_` vars map).
- **New dependencies**: `templ` (codegen step), `golang.org/x/crypto/bcrypt`, the Tailwind standalone binary (build-time only), and vendored templ UI components.
- **Build pipeline**: `go build` is preceded by `templ generate` + `tailwindcss`; build scripts/CI updated accordingly. Output remains a single binary (embedded assets).
- **Reused without change**: `credential.Store.ListMasked`, `config.LoadOrRefreshCatalog` / `FetchProviderModels`, `history.Querier.List`, and the topology mutators.
- **Security surface**: a new password-gated, browser-facing write surface on loopback. Requires the CSRF/origin guard and login throttle; all secret fields are write-only/masked in both the UI and JSON serialization (no plaintext token leakage, per `security.md`).
