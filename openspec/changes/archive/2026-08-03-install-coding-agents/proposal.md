## Why

tinyroute already mints API keys and serves each dialect on a namespaced path
(`/anthropic/v1/messages`, `/openai/v1/responses`, …), but pointing a downstream coding agent
(Claude Code, Codex, Cline, …) at the gateway is a manual, error-prone chore: the user must know
which file to edit, which env vars or TOML fields to set, and the correct base URL — and the only
built-in hint (`keys create`) is itself path-imprecise (it prints `http://<listen>`, omitting the
`/<dialect>` prefix the mounted routes require). We should make agent onboarding a one-command,
correct-by-construction operation, as 9router already does with its per-agent "CLI tool" adapters.

## What Changes

- Add an **agent adapter registry** (`internal/agent/`) where each adapter knows how to read,
  merge-write, and reset one coding agent's config file(s) under the user's home directory.
- Add a `tinyroute agent` command group — `agent install`, `agent status`, `agent uninstall` —
  following the interactive-first CLI pattern (args-as-escape-hatch, prompts-as-default).
- On install, derive the agent's base URL from `svc.Listen` + the agent's dialect, and mint a
  fresh scoped `tr_live_` key (or accept a caller-supplied token) written into the agent config.
- Port 9router's full config-write adapter set: `claude`, `codex`, `cline`, `copilot`,
  `deepseek`, `devin`, `droid`, `grok`, `hermes`, `jcode`, `kilo`, `openclaw`, `opencode`.
- Defer two 9router adapters whose mechanism is not a config write: `cowork` (MCP registry) and
  `antigravity-mitm` (MITM proxy interception) — out of scope, documented as future work.
- Writes are safe by construction: backup existing config, **merge** (preserve unrelated user
  fields), atomic `tmp+rename` at `0600`, idempotent re-runs, and uninstall removes only the
  fields tinyroute injected.

No **BREAKING** changes. Existing commands, routes, and key behavior are untouched; the agent
feature only adds new surface and reuses `auth.GenerateKey`/`auth.WriteKeyFile` as-is.

## Capabilities

### New Capabilities

- `agent-install`: Writing/reverting coding-agent config files so a chosen agent points at the
  tinyroute gateway — the adapter registry, the per-agent config contract (paths, fields, reset
  lists), and the `agent install | status | uninstall` behavior.

### Modified Capabilities

None. The feature reuses `api-keys` (key creation + `Allow` scoping) and `interactive-prompts`
without altering their requirements.

## Impact

- **New code:** `internal/agent/` package (interface, registry, shared format helpers, one file
  per adapter + tests); `internal/cli/agent.go` (command group + handlers + tests).
- **Modified code:** `internal/cli/cli.go` — register `cmdAgent()` in the root command list.
- **New dependencies (pending design decision):** a TOML library (`github.com/pelletier/go-toml/v2`)
  for the TOML-provider family and a YAML library (`gopkg.in/yaml.v3`) for `hermes`; JSON-family
  adapters use the stdlib. Alternative (hand-rolled writers) evaluated in `design.md`.
- **No API/route changes:** agents are configured to hit the already-mounted dialect paths; no
  new inbound endpoints are introduced.
- **Filesystem:** writes are confined to well-known paths under the user's home directory
  (`~/.claude/`, `~/.codex/`, …), each backed up before modification.
