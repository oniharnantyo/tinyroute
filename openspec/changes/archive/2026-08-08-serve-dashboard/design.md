## Context

tinyroute's `serve` command builds a single `http.Server` on a loopback listener (default `127.0.0.1:8787`) whose mux carries only dialect mount paths (`/anthropic/v1/*`, `/openai/v1/*`, …) plus per-dialect `/models` endpoints. The mux is wrapped by an access-log middleware; proxy requests flow through `proxy.Handler`, which records into the SQLite history store. All management today is via the CLI (`provider add`, `model add`, `auth set`, …) which mutates `config.json` through `config.ParseRawTopology` → `config.WriteTopology` (atomic, mode `0600`, preserves `${VAR}` interpolation) and `keys.json`/`credentials.json` through dedicated stores. Existing read APIs already expose everything a UI needs: `topologyWatcher.Get()`, `route.HealthStore`, `history.Querier.List`, `credential.Store.ListMasked`, `config.LoadOrRefreshCatalog` / `FetchProviderModels`. No web framework is currently a dependency.

## Goals / Non-Goals

**Goals:**
- A loopback-served, password-gated web UI that observes live gateway state and manages topology/keys, opened automatically by `serve`.
- Reuse existing mutators and read APIs verbatim — no parallel write/serialization path.
- Stay node-free and single-binary (embedded assets).
- Honor the repo's security rules: secret masking, atomic `0600` writes, CSRF defense, no plaintext leakage.

**Non-Goals:**
- Multi-user accounts / RBAC (single shared dashboard password only).
- Binding the dashboard beyond loopback.
- A separate dashboard process or port (single shared listener).
- Re-implementing provider/model/key logic that the CLI already covers.

## Decisions

### D1. Shared gateway port, not a separate management listener
Dashboard routes mount under `/dashboard/*` on the existing `serve` mux. **Rationale:** one address, one server, one lifecycle — simplest story for a local tool. Dashboard handlers are registered directly on the mux and deliberately do **not** route through `proxy.Handler`, so dashboard traffic is never recorded into request history (the "don't pollute history" property falls out structurally). The mux's access-log middleware still wraps dashboard requests (desirable — dashboard activity is logged to stderr).
- *Alternative considered:* a second `http.Server` on a dedicated port. Cleaner separation (independent bind, no access-log wrap), but rejected for simplicity; reversible later since dashboard handlers are self-contained.

### D2. templ + Tailwind standalone binary (node-free)
UI is built with templ (Go-native, `templ generate` codegen) and Tailwind compiled by the single-file `tailwindcss` binary; the compiled CSS is embedded via `go:embed`. Output remains one binary.
- *Alternatives considered:* Tailwind via npm/bun (rejected — introduces node into a pure-Go repo and CI); templ + hand-written CSS (rejected — forfeits the templ UI v2 component library, whose components are Tailwind-class-based).

### D3. Password auth (bcrypt), default `123456`, persisted and changeable
Auth is a single dashboard password. On first run, `~/.tinyroute/dashboard.json` is seeded with a bcrypt hash of `123456` (mode `0600`, atomic write). Login sets a `SameSite=Strict` session cookie; the password is changeable from a settings screen. **Rationale:** the requirement that the password persist and be changeable in-UI rules out an ephemeral bootstrap token. bcrypt is mandatory because a user-chosen password is low-entropy and often reused (unlike the high-entropy tokens in `credentials.json`).
- *Alternatives considered:* ephemeral one-shot bootstrap token (rejected — can't persist/change); reuse `keys.json` with an `admin` capability (rejected — conflates proxy-call authority with management authority); no auth (rejected — a manage surface on loopback is too exposed to local-process/browser-mediated attacks).

### D4. Default password stays active until manually changed (accepted risk)
No forced first-login change. **Rationale:** explicit product choice for a friction-free first run. **Mitigations:** loopback-only bind, login throttle (D6), CSRF/origin guard (D6), and a persistent in-UI nudge chip. This is recorded as an accepted risk, not a gap.

### D5. Reuse existing mutators and read APIs — never marshal topology directly
All manage actions go through `ParseRawTopology` → mutate struct → `WriteTopology`. **Rationale:** this is the only path that preserves `${VAR}` interpolation and atomic `0600` writes; a naive `json.Marshal` of the interpolated topology would bake literal secrets into `config.json`. Connections read via `credential.Store.ListMasked()` (masked by construction); models via `LoadOrRefreshCatalog` / `FetchProviderModels`; history via `Querier.List`. The dashboard is a second front-end to one capability — if the CLI can't do it yet, the dashboard doesn't either.
- *Consequence:* the model test action requires lifting the probe logic currently inlined in `cmdProviderModelTest` into a shared helper (e.g. a `probe` function) so both CLI and dashboard call it.

### D6. CSRF / DNS-rebinding defense + login throttle
Mutating POSTs are rejected unless the `Host` header is a loopback host (`localhost`/`127.0.0.1`), in addition to the valid `SameSite=Strict` session cookie. Login attempts are throttled with the existing `auth.RateLimiter`, keyed on the client address. **Rationale:** a browser-facing write surface on loopback is specifically vulnerable to DNS-rebinding / cross-origin localhost fetch driving the user's authenticated browser.

### D7. Model cards are lean: two names + test + copy + remove
Each whitelisted-model card shows only the **tinyroute model name** (`provider:model`, the form `Router.Models()` exposes and clients request), the **provider model name** (the whitelist/upstream ID), a copy control, a Test action (D5 probe), and a remove control. **Rationale:** explicit UI spec; it also means **no catalog schema change is needed** — both names are already available (`Provider.Models` + trivial prefix), so the earlier-considered rich `ModelInfo` (pricing/capabilities/context) extension is dropped as unnecessary.

### D8. Icons: Lucide via templ UI `icon`, never emoji
All icons (status, nav, actions) render as SVG through templ UI's `icon` component (Lucide, embedded in `icon_data.go`). Provider brand logos are custom SVGs via `go:embed` (count as real icons). **Rationale:** consistent cross-platform rendering, Tailwind color inheritance, proper a11y labels, and node-free embedding. Components (including the Lucide-backed `icon`) are vendored via the templ UI CLI preset `templui apply --preset b2DKLWUNe` rather than a hand-maintained icon map — the mechanism that delivers the real templ UI `icon` and retires the hand-rolled `internal/dashboard/components/icons.go` (verify suggestion #9).

### D9. Providers: compact list card → click-through detail view
The providers page splits into a **compact list** (each card shows only logo + display name + connection count) and a **detail view** at `/dashboard/providers/{name}` (header, Connections, Models, read-only Advanced). **Rationale:** the prior single inline card crammed summary and detail together — why logos went unused and "connections" collapsed to a boolean (verify findings #5, #6) and model-add was free text (finding #4). The split makes logos primary on the card, forces a real per-provider connection count, and gives the catalog/live model picker and the masked-connections list a proper home; it also matches the existing history row→drawer summary/detail pattern. **Connection count** = `len(Accounts)`, else 1 if a direct `APIKey` is set, else 0; detail connections come from `Provider.Accounts` enriched with `credential.Store.ListMasked()` (masked). The whitelisted-models section is renamed to **Models**.

### Package / route layout (new `internal/dashboard/`)
```
internal/dashboard/
  handler.go      route registration on the shared mux (/dashboard/*)
  auth.go         bcrypt password store, session cookie, login/throttle, Host/Origin guard
  view_*.templ    pages (login, overview, providers, routes, history, keys, settings)
  components/     templ UI vendored components + icon
  assets/         go:embed compiled CSS + provider logo SVGs
serve.go          conditional dashboard mount + browser-open + shutdown, gated by --no-dashboard
config/service.go new Dashboard fields + flag + known-TINYROUTE_ vars
```

## Risks / Trade-offs

- **Default password `123456` remains live** → mitigated by loopback bind, login throttle, CSRF/origin guard, in-UI nudge chip (D4).
- **`WriteTopology` reformats `config.json` on save** (sorted keys, normalized indentation) → minor DX surprise for users who hand-edit; documented, not blocking.
- **New build steps** (`templ generate`, `tailwindcss`) → build scripts/CI must run them before `go build`; single-binary output preserved via `go:embed`.
- **Provider logo trademarks** → use official SVGs with a generated monogram fallback for any without one; low-risk for a local-first tool.
- **Dashboard traffic flows through the access-log middleware** → acceptable/desirable (dashboard activity logged); it does *not* reach the history recorder.

## Migration Plan

- **Purely additive:** new `internal/dashboard/` package and a new `--no-dashboard` flag (default on). The existing CLI and proxy paths are untouched.
- **No data migration:** `~/.tinyroute/dashboard.json` is created on first `serve` if absent (seeded with the default-password hash). Existing `config.json`/`keys.json`/`credentials.json` are read/written only through the existing stores/mutators.
- **Rollback:** run `serve --no-dashboard` (or `TINYROUTE_DASHBOARD=false`); the dashboard is fully opt-out with no side effects on the proxy.

## Open Questions

- **Provider logos:** official SVGs vs monogram-only (brand/trademark stance). Leaning official + monogram fallback; finalize during implementation.
- **Future separate bind:** whether to later expose the dashboard on its own listener (D1 is deliberately reversible).
