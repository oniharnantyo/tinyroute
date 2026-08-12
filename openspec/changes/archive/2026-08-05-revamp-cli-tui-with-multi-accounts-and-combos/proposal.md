## Why

The CLI's interaction model is a one-shot command bus (`urfave/cli/v3` + `pterm` prompts): run a command, get a table, exit. It cannot express two capabilities users actually need: (1) a **provider holding many credentialed accounts** (several OpenAI API keys, or several codex OAuth accounts), and (2) the router resolving a *logical model* through an ordered/pooled/fused panel — the "combo" model proven in the `9router` reference. The interactive `init` wizard is also linear (single provider, no back-nav), which the full-screen TUI fixes. This change converges all three: a navigable full-screen TUI, a first-class multi-account model, and full combo capabilities (named combos + capability reorder + fusion).

## What Changes

- **New `tinyroute tui` full-screen navigator** (Bubble Tea): sidebar navigation (Providers, API Keys, Routes/Combos, Sessions, Agents, Status, Live Logs) with panes, modals, and function-key actions. The current per-command table output and `--no-interactive` paths remain intact and scriptable.
- **`tinyroute init` interactive wizard replaced by a full-screen setup app** (same transactional save: nothing on disk until final confirm; non-TTY falls back to the existing scaffold). Multi-provider selection and back/forward navigation are added.
- **Multi-account providers**: a `Provider` gains `accounts []Account` (each a named credential: static key or OAuth), plus a `selection` strategy (`round_robin` | `fill_first` | `sticky`). Legacy single `api_key`/`credential` becomes an implicit default account (backward compatible).
- **`credentials.json` re-keyed to `provider/account`** (secret store stays separate from config; tokens never leave it). Existing single records migrate to a `default` account.
- **Account selection + failover at runtime**: per hop the proxy picks an account via the strategy, and per-account cooldown/backoff (extending `route.HealthStore` to `provider/account`), pivoting to the next account on failure.
- **Routeable accounts** (`provider@account:model` pins one account; `provider@default:model` uses the pool).
- **Combos**: named logical models expand to `[provider/model…]`, with **capability reorder** (vision/pdf/audio/video hard caps tier the panel) and **fusion** (parallel fan-out, quorum, judge-model synthesis, streaming). This is the largest slice.
- **Agents** can carry multiple OAuth accounts (e.g. several codex accounts) managed through the provider UI, rotating across them.
- **Sessions/status/logs** surface which account served each request.

## Capabilities

### New Capabilities
- `provider-accounts`: a provider holds an ordered, policy-selected set of credentialed accounts; selection (`round_robin`/`fill_first`/`sticky`) and per-account cooldown/failover.
- `model-combos`: named logical-model combos resolving to an ordered/pooled/fused panel, with capability reorder.
- `cli-tui-navigator`: the full-screen navigator shell (sidebar, panes, modals, key bindings) plus the full-screen setup app replacing the init wizard.

### Modified Capabilities
- `provider-credentials`: credentials keyed by `provider/account`; runtime account selection; per-account refresh dedup/cache.
- `provider-registry`: `Provider` schema gains `accounts` + `selection` (legacy fields kept as the default account).
- `core-routing`: resolver handles `provider@account` and combo names; capability reorder; model discovery lists combos.
- `interactive-wizard`: replaced by the full-screen setup app.
- `agent-install`: multiple OAuth accounts per agent (codex etc.), rotating across them.
- `session-history`: records the account (provider/account) that served each request/replay.

## Impact

- **Code**: `internal/config` (Topology/Provider/Account/Route schema, parsing/validation), `internal/credential` (store re-keying, account-aware selection), `internal/route` (health per account, account selector), `internal/proxy` (attempt loop picks accounts; fusion execution path), `internal/cli` (new `tui/` package; `init` reroute), `internal/agent` (multi-account), `internal/history/sqlite` (account field), `internal/core` (new types: hop account, combo, selection).
- **Dependencies**: add `charmbracelet/bubbletea`, `charmbracelet/bubbles`, `charmbracelet/lipgloss` for the TUI.
- **Data**: `config.json` (accounts/selection/combos, additive), `credentials.json` (composite keys, migrated), `state.json` (cooldown keys), `history.db` (new column).
- **BREAKING (minor)**: `credentials.json` key format changes from `provider` to `provider/account` (auto-migrated). Everything else is additive.
