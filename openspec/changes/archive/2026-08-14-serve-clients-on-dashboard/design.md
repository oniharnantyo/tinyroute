## Context

tinyroute is an LLM gateway. `internal/agent` (to be renamed `internal/clients`) is a
self-registering registry of 13 adapters that configure downstream coding clients
(Claude Code, Codex, Cline, …) to route through the gateway by writing their config
files (`~/.claude/settings.json`, `~/.codex/config.toml`, …). Today the registry is
imported by exactly one file — `internal/cli/agent.go` — and the install orchestration
there is welded to `interactive.*` (`Select`/`Password`/`Confirm`/`Input`/`MultiSelect`),
i.e. stdin/TTY. The dashboard (`internal/dashboard`, templ + Tailwind + Alpine, dark
slate) has no clients surface. Two specs govern this area: `agent-install` (written
entirely in TTY terms) and `management-dashboard` (which has no clients requirement).

Hard constraints the dashboard already enforces: SVG icons only (no emoji), masked
secrets, atomic writes at POSIX `0600`, and loopback-only mutations
(`HostGuardMiddleware`). Coding-style rules require interface / DTO / implementation to
live in separate files, and the CLI is interactive-first (prompts-as-default,
args-as-escape-hatch).

## Goals / Non-Goals

**Goals:**
- Rename `agent` → `clients` across code, CLI, and specs with **no behavior change**.
- Make install orchestration a reusable, TTY-free core so the CLI and the dashboard
  drive the same flow.
- Add a **Clients** dashboard surface: a list with connection-derived status badges,
  and a per-client live configuration editor (endpoint, masked key, model slots,
  context window) with install preview → confirm → one-time key reveal, uninstall, and
  a manual-config snippet.

**Non-Goals:**
- The "filter naming requests" toggle (undefined — deferred until specified).
- Web search / Exa MCP integration (substantial; a separate change).
- Relocating static provider API keys into the credential store.
- Changing the on-disk config formats the adapters write.
- Renaming archived changes (historical record — left untouched).

## Decisions

**1. Term: "clients".**
"agent" is overloaded in the LLM space; "tool" collides with the gateway's
function-calling translation domain (151 `tool` refs across `internal/translate` and
`internal/core`: `tool_calls` ↔ `tool_use` ↔ `functionCall`). "clients" is accurate
(they are HTTP clients of the gateway), covers both CLIs and editor plugins, and has
zero collision. *Alternatives considered:* "CLI Tools" (matches the reference wireframe
but is inaccurate for editor plugins like Cline/Copilot/Kilo, and still risks grep
collision); "Integrations"/"Apps" (generic).

**2. One combined change, rename as the first task block.**
The rename is behavior-preserving and independently verifiable, but the dashboard
cannot reference `clients.*` until it lands, so the rename is task block 1 within this
change. *Alternative considered:* two changes (rename, then dashboard) — cleaner
review boundaries but more ceremony; rejected for momentum.

**3. Extract `clients.Installer` (`Plan` / `Apply` / `MintKey`).**
Pure logic moves out of `internal/cli/agent.go` (`dialectBaseURL`,
`discoverModelsForDialect`, mint-vs-reuse, the `Apply` call) into a TTY-free type.
`Plan` returns a structured preview without writing; `Apply` writes on a confirmed
plan; `MintKey` mints a dialect-scoped key. The CLI becomes a thin adapter (gather
inputs via prompts → `Plan` → render text → confirm → `Apply`); the dashboard does the
same over HTTP. *Alternative:* fork the logic in the dashboard — rejected (two sources
of truth, drift).

**4. Live editor via extended `Detect()`.**
The reference wireframe is a read-modify-write editor, so `Detect()`/`Status` grows to
return the client's *current* base URL, a masked key digest, and per-slot model values
(read from the client's own config file). The tinyroute keystore still stores only
digests — we mask the value already present in the tool's config, never inverting a
digest. *Alternative:* install-only wizard — rejected (cannot show/edit current state).

**5. Minted-key one-time reveal: direct-render, not PRG.**
`Apply` that mints a key returns a result page directly (with a copy control), not a
redirect. This breaks pure PRG for this one endpoint but keeps the plaintext out of
redirect URLs, flash params, and logs. *Alternative:* session-stash + redirect reveal
page — more moving parts, rejected for simplicity.

**6. Endpoint is a dropdown of gateway dialect URLs; default derived.**
Options are the dialect-mapped gateway endpoints (`<listen>/anthropic`,
`<listen>/openai/v1`, `<listen>/gemini`); the default is the one derived from the
client's dialect. A read-only **Current** field shows what the client points to today.

**7. Preview/confirm is server-authoritative.**
Selections POST → server returns the `Plan` → an Alpine modal renders it → confirm POSTs
→ `Apply`. The server, not the client, computes the preview (matches the CLI, avoids the
browser diverging on base-URL/key logic).

**8. Blank-import the adapters in `serve.go`.**
Adapters self-register via `init()`; only `internal/cli/agent.go` imports them today, so
`clients.All()` would be empty in a `serve` process. A blank import on the serve path
(mirroring the dialect blank-imports at `serve.go:24-27`) populates the registry.

**9. Model-slot rows are generated from `ModelSlots()`.**
The editor renders every declared slot (single → bounded picker, multi → multi-select),
options from the models routable on the client's dialect. All slots render, including
claude's `subagent` (not shown in the reference wireframe but present in the contract).

## Risks / Trade-offs

- **[Masked-key display adds file reads to `Detect()`]** → masking happens in the view
  layer; the plaintext is never logged or emitted in JSON. `Detect()` already reads the
  config file, so this is an additional field parse, not new I/O class.
- **[Host-local surprise on remote dashboard access]** → install/uninstall configure the
  *gateway host's* files. Mitigated by an explicit UI callout ("configures clients on
  this machine") and the existing loopback-only mutation guard.
- **[Rename blast radius — 13 adapters, CLI, tests, specs]** → purely mechanical;
  gated by `go build ./...` and `go test ./...`. Archived changes are untouched.
- **[Capability folder rename `agent-install → clients-install` is not a delta op]** →
  the spec delta is written under the existing `agent-install/` folder; the folder move
  is a tracked task, reconciled with `openspec validate` before archive.

## Migration Plan

1. **Rename** (`internal/agent` → `internal/clients`, CLI, specs) — `go build`/`go test`
   green; `tinyroute clients` behaves identically to the old `tinyroute agent`.
2. **Extract `Installer`** — CLI delegation with parity tests proving unchanged output.
3. **Dashboard handlers + views** — list, editor, preview/confirm, reveal, uninstall.
4. **Wiring** (`serve.go` blank-import) + full verify (`gofmt`, tests, `openspec validate`).

Rollback is a plain revert; there is no data migration (on-disk config formats are
unchanged).

## Open Questions

- **"Filter naming requests"** — needs a definition before it can be specified
  (deferred from this change).
- **Exa MCP / web search** — scope and ownership (separate change).
- **`subagent` slot** — default decision is to render all declared slots; confirm at
  implementation.