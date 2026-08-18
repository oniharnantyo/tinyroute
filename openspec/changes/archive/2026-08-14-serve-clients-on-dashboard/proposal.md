## Why

Configuring a downstream coding client (Claude Code, Codex, Cline, …) to route
through tinyroute is the **only management surface still locked to the CLI/TTY** —
every other surface (providers, keys, routes, history) is doubled across CLI and
dashboard. A user running `serve` locally gets a browser dashboard but must drop to a
terminal to point a client at the gateway, and has no way to *see* which clients are
already connected. The blocker is structural: the install orchestration in
`internal/cli/agent.go` is welded to `interactive.*` (stdin/TTY), so it cannot be
reused from an HTTP handler. Surfacing this on the dashboard requires both a rename to
a non-colliding term ("clients" — "agent" is overloaded, "tool" collides with the
gateway's function-calling translation domain) and a TTY-free core shared by CLI and
dashboard.

## What Changes

- **Rename `agent` → `clients` everywhere** (package `internal/agent` →
  `internal/clients`; interface `Agent` → `Client`; CLI `tinyroute agent` →
  `tinyroute clients`; specs `agent-install` → `clients-install`, and the
  `cli-tui-navigator` "Agents" category → "Clients"). Behavior-preserving; archived
  changes are left as historical record. The target config paths adapters write
  (`~/.claude/settings.json`, `~/.codex/config.toml`, …) are unaffected.
- **Extract a TTY-free install core** (`clients.Installer`: `Plan` / `Apply` /
  `MintKey`) shared by the CLI and the dashboard. The CLI handlers become thin
  adapters with unchanged observable behavior; a non-TTY caller satisfies preview +
  confirmation through the structured `Plan`.
- **New "Clients" dashboard surface** mirroring `tinyroute clients`:
  - A **clients list** (`/dashboard/clients`) of cards — one per registered client —
    each badged **Connected** / **Not Configured** / **Not Installed**, derived from
    `Detect()`.
  - A **client detail / live editor** (`/dashboard/clients/{id}`) matching the
    reference wireframe: an in-page client switcher, an endpoint dropdown (gateway
    dialect URLs, default derived), a read-only **Current** endpoint, a masked **API
    key**, per-slot model pickers generated from `ModelSlots()` with Select/clear,
    and a **Context window** field. Actions: **Apply** (preview → confirm → write,
    with one-time minted-key reveal), **Reset** (uninstall), **Manual config** (render
    the snippet to paste).
- **Extend `Detect()`** to return the client's *current* endpoint, masked key, and
  per-slot model values, so the editor renders live state (read-modify-write), not
  just install booleans.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `management-dashboard`: gains a **Clients** management surface — a clients list with
  connection-derived status badges, and a per-client live configuration editor
  (endpoint, masked key, model slots, context window) with install preview/confirm,
  one-time minted-key reveal, uninstall, and a manual-config snippet.
- `agent-install`: **renamed to `clients-install`** and gains a requirement that
  install orchestration is a reusable, medium-agnostic core (`Plan`/`Apply`/`MintKey`)
  consumed by both the CLI and the dashboard, so the flow is no longer TTY-bound.
- `cli-tui-navigator`: the "Agents" command category and the `agent` command are
  renamed to "Clients" and `clients`.

## Impact

- **Code:**
  - `internal/agent` → `internal/clients` — package, `Agent`→`Client` interface,
    `Register/Get/All`, `Status`/`ApplyInput`/`Result`/`ModelSlot`, and the 13 adapter
    files (`package clients`). `Status` grows fields for current endpoint / masked key
    / slot values.
  - NEW `internal/clients/installer.go` (+ `_test.go`) — TTY-free `Installer`; pure
    logic moved out of the CLI (`dialectBaseURL`, `discoverModelsForDialect`,
    mint-vs-reuse, `Apply`).
  - `internal/cli/agent.go` → `internal/cli/clients.go` — `cmdClient*` thin adapters
    over `Installer`; `tinyroute clients install|status|uninstall`.
  - `internal/dashboard/handler.go` — new `handleClientsView`,
    `handleClientDetailView`, `handleClientInstall` (preview → apply), and
    `handleClientUninstall`; new view-data types; `Deps` gains installer needs (wired
    in `serve.go`).
  - NEW `internal/dashboard/view_clients.templ`, `view_client_detail.templ` (+ gen).
  - `internal/dashboard/view_layout.templ` — add the **Clients** nav item (Lucide SVG
    icon; no emoji).
  - `internal/cli/serve.go` — blank-import the client adapters so `clients.All()`
    populates in the serve process.
- **Specs:** `management-dashboard` (+Clients requirement), `agent-install`
  (→`clients-install`, +reusable-core requirement), `cli-tui-navigator` (category
  rename).
- **Tests:** `internal/clients/installer_test.go`; dashboard handler/view tests
  (list badges, editor live-state, preview→confirm→apply, one-time reveal, uninstall);
  `internal/cli/clients_test.go` asserting CLI parity through the shared core.
- **Untouched:** routing/proxy, dialects, translate, history; clients write config
  files on the gateway host and do not change request handling. Topology format and
  `${VAR}` references are preserved.
- **No breaking config changes.**