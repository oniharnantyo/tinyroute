## Context

tinyroute serves dialect-namespaced inbound paths (`/anthropic/v1/messages`,
`/openai/v1/responses`, …) and issues **digest-only** API keys, but has no first-class way to
wire a downstream coding agent to the gateway. 9router (the sibling reference at `9router/`)
implements this as a per-agent adapter set (~15 "CLI tools") that write JSON/TOML/YAML configs to
well-known home-directory paths, merging with existing user settings and reversible on uninstall.

This design ports that pattern into tinyroute's Go CLI under two hard constraints:
1. **Digest-only key storage** (`internal/auth/keystore.go`) — plaintext keys cannot be replayed.
2. **Dialect-namespaced routes** — each agent's base URL is a function of `(listen, dialect)`.

Reference per adapter: `9router/src/app/api/cli-tools/<id>-settings/route.js` +
`9router/src/components/<X>ToolCard.js`.

## Goals / Non-Goals

**Goals:**
- One command (`agent install <agent>`) that correctly configures a supported agent to route
  through tinyroute, interactive-first.
- An adapter registry extensible without touching CLI or core code.
- Safe, reversible config writes: backup → merge → atomic → idempotent → scoped reset.
- Cover 9router's 13 config-write adapters.

**Non-Goals:**
- `cowork` (MCP server registry) and `antigravity-mitm` (privileged MITM proxy) — different
  mechanisms, deferred.
- A dashboard/UI — tinyroute is CLI-only; we port the *config-write* behavior, not 9router's web.
- Auto-launching or auto-updating agent binaries — only detect presence for `status`.
- Changing the `api-keys`, `core-routing`, or `interactive-prompts` specs.

## Decisions

### D1. Adapter interface + registry (not a switch statement)
Per-agent variance (path, format, fields, reset list) is high. An interface keeps each adapter
isolated and independently testable, and adding an agent is one new file + one registry entry —
mirroring `internal/dialect/registry.go` and `internal/preset/`.
*Alternative considered:* a single `switch(id)` in one file — rejected as a ballooning,
untestable file that violates the repo's "separate contract/types/impl" rule.

### D2. Base URL derived per-dialect from `svc.Listen`
tinyroute mounts each dialect at `/<dialect>/...`, so the base URL is a function of
`(svc.Listen, dialect)`, not a per-agent constant. One helper (`dialectBaseURL`) sources the
listen address from `config.Service` (single source of truth).
*Alternative:* hardcode per adapter — rejected; duplicates the dialect→path mapping and drifts
from the actual mount paths (the very bug in today's `keys create` hint).

Note the openai *family*: the `openai` (chat) and `openai-responses` dialects both mount under
`/openai/v1/…` (`/openai/v1/chat/completions` and `/openai/v1/responses`), so an agent speaking
either variant gets the same `/openai` base — the client appends `/chat/completions` or
`/responses`. The dialect *name* (`openai-responses`) still drives auth key scoping
(`Allow=["openai-responses:*"]`), but the URL path family is `openai`. Conflating the two — e.g.
using the dialect name as the path prefix (`/openai-responses/…`) — produces a URL tinyroute does
not serve (the historical `codex` defect).

### D3. Mint a scoped key per install, or reuse a caller-supplied token — user's choice
Because keys are stored as sha256 digests only, an existing key's plaintext cannot be replayed
into an agent config. Install therefore offers the user a choice: **mint** a fresh scoped key
(`Key.Allow = ["<dialect>:*"]`, plaintext available once), or **reuse** a token they supply
(pasted with masked input, or passed via `--api-key`). The interactive flow prompts this choice
explicitly; `--api-key` pre-selects reuse, and non-interactive runs default to minting. Either
way the chosen token is deferred until after the preview/confirm gate (D9), so an abort mints
nothing.
*Alternative:* store plaintext to enable replaying an existing key — rejected; violates the
security rule (zero plaintext at rest) and weakens the keystore invariant.

The minted key's display `Name` defaults to `agent-<id>`; the user may override it via an
interactive prompt or `--name`, so keys are distinguishable in `keys list` (e.g.
`claude-code-laptop`).

### D4. TOML via `pelletier/go-toml/v2`, YAML via `gopkg.in/yaml.v3`, JSON via stdlib
Merging into a user's existing config while preserving unknown fields requires
parse → mutate → marshal of a **generic map** (not a struct, which drops unknowns). The stdlib
covers JSON. TOML/YAML round-trip of `map[string]interface{}` needs libraries; go-toml v2 and
yaml.v3 both do this and match 9router's `confbox` approach.
*Alternative:* hand-rolled writers — rejected for TOML (nested `[model_providers.X]` + root
scalars make lossless merge error-prone); feasible for hermes' 2-line YAML but rejected for
consistency.
*Trade-off:* two new deps vs. tinyroute's "tiny" ethos — **accepted**, because correctness on
user-owned config files outweighs dependency count, and both libs are pure-Go and ubiquitous.

### D5. Format-family shared helpers (`envjson.go`, `tomlprovider.go`)
`claude`/`droid`/`openclaw`/`opencode` share the "JSON settings with an env/keys sub-object"
shape; `codex`/`deepseek`/`grok`/`jcode` share the "TOML `[model_providers.X]` + root fields"
shape. Shared helpers collapse each adapter to path + field-name wiring and centralize the
round-trip/merge tests.
*Alternative:* each adapter fully hand-rolled — rejected as duplicated and drift-prone.

### D6. Write safety: backup → merge → atomic(0600) → idempotent; scoped reset
Editing user-owned config files is high-blast-radius. Backup (`.tinyroute.bak`) + merge
(preserve unrelated fields) + atomic `tmp+rename` (no half-writes) at mode `0600` + scoped reset
(port each 9router `RESET_*` field list) minimize risk. This reuses the existing
`auth.WriteKeyFile` / `config.WriteTopology` idiom and satisfies the security rule.
*Alternative:* overwrite — rejected (destroys user config).

### D7. Defer `cowork` and `antigravity-mitm`
`cowork` installs MCP server entries (a registry, not a base-url/key config);
`antigravity-mitm` intercepts traffic via a privileged MITM proxy. Neither fits the "write agent
config pointing at a URL" model. They are documented as future work and **not** offered by the
registry.

### D8. Model selection via the `ModelSlots()` interface method
Model handling across the 9router adapters is not uniform — four shapes: **none** (devin,
binary-managed auth), **single** (cline, deepseek, hermes, kilo), **single + subagent** (codex,
grok), **tiered named singles** (claude: fable/opus/sonnet/haiku + subagent), and **multi-list**,
optionally with an active/default and/or subagent (copilot, droid, jcode, opencode, openclaw).
Rather than special-case each, every adapter implements `ModelSlots() []ModelSlot` as a **required**
method on the `Agent` interface (adapters with no model config — devin — return `nil`). Each
`ModelSlot{ID, Name, Kind, Required}` is either a single pick (`Kind == SlotSingle`) or a
multi-list (`Kind == SlotMulti`). Claude's tiers are five single slots; codex/grok are two single
slots; copilot/droid/jcode/opencode/openclaw use a multi slot (some plus a companion single for
active/subagent). The install flow prompts each slot (`Select` or `MultiSelect`) from the models
routable on the agent's dialect. Selections flow through `ApplyInput`: `ModelSlots`
(`map[string]string`, slot-ID → model) for named slots, plus `Model` (primary single) and `Models`
(`[]string`, multi list) for adapters using those; each adapter maps its slot IDs to its own config
fields per the per-agent contract in `specs/agent-install/spec.md`.
*Why a required method, not a separate optional interface:* every adapter already implements
`Agent`, so a required `ModelSlots()` (nil for none) is simpler than an optional capability the
install flow must type-assert. The cost is one trivial method on non-model adapters.

### D9. Preview + confirm; defer key minting until after confirmation
Interactive installs render a preview (agent, resolved base URL, key source, selected model(s),
target path(s), backup note) and require explicit confirmation before writing. To make
"abort = no side effects" literally true, the API key is **not** minted until the user confirms —
only the *decision* (mint vs supplied) appears in the preview. Non-interactive runs
(`--force` / `--no-interactive`) skip the preview and apply directly, preserving scriptability.
*Alternative:* render a full file diff — rejected for v1 (needs a dry-run `Preview()` method on
every adapter); the summary preview is sufficient and avoids interface churn. A diff can land later
behind the same confirmation gate.

## Risks / Trade-offs

- **[Upstream agent changes its config schema]** → each adapter is isolated behind the interface
  with golden-fixture tests; the 9router `route.js` is the noted source of truth. A schema change
  is a local, single-file fix.
- **[TOML/YAML round-trip drops unknown user fields]** → parse-to-map + mutate + marshal-map;
  golden tests assert an arbitrary user config survives install *and* uninstall unchanged outside
  the injected fields.
- **[Plaintext key written to agent config on disk]** → file mode `0600`, backup also `0600`, key
  scoped to one dialect; same exposure surface as `keys create` (which already prints the key).
  Accepted; documented in `agent status`/install output.
- **[Path/platform variance]** (kilo uses `~/.local/share` + VS Code settings; XDG on Linux) →
  per-adapter path resolution ports 9router's logic verbatim; tests use `t.Setenv("HOME", …)`.
- **[Two new dependencies]** → accepted per D4; both pure-Go, actively maintained.
- **[No routed models for a dialect]** → the model picker falls back to free-text entry for a
  required selection and skips optional tiers (per the Model selection requirement); the user is
  never blocked by an empty picker, and an install with no routing configured still sets base URL +
  key.
- **[Per-agent schema drift / unverified adapters]** → official-doc verification surfaced concrete
  corrections and unverified tools: **copilot** must use `vendor: "customendpoint"` (not `"azure"`)
  and drop the `#models.ai.azure.com` URL fragment; **kilo** should target the documented
  `kilo.json` (the `kilocode.*` VS Code keys are undocumented); **cline** writes `globalState.json`,
  an internal extension store that is not an officially hand-editable file (fragile across Cline
  versions); **droid** writes undocumented `id`/`index` fields (likely harmless). **deepseek,
  grok, hermes, jcode, openclaw** could not be verified against trustworthy official docs — their
  configs are taken from 9router and need human confirmation before release.

## Migration Plan

Additive feature; **no migration**. Rollback: `agent uninstall <agent>` reverts each configured
agent; the `.tinyroute.bak` backups remain. Removing the package leaves existing configs
untouched. No changes to `keys.json`, `config.json`, routes, or mounted paths.

## Open Questions

- Exact config format for `copilot` and `devin` — confirm from their 9router `route.js` during
  implementation.
- Confirmed dialect for `opencode` and `openclaw` (both are multi-provider) — pick the dialect
  tinyroute exposes and note it in the adapter.
- Whether to also correct the imprecise base URL in `keys create` output — out of scope here;
  tracked separately.
