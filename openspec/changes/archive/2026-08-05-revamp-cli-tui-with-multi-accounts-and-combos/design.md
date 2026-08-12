## Context

`tinyroute` is a Go 1.26 LLM gateway that proxies `provider:model` inference
requests to upstream providers. Today it is a one-shot command bus:

- **CLI** (`internal/cli`): `urfave/cli/v3` command tree + `pterm` prompts. Each
  command prints a table (`text/tabwriter`) and exits (`commands.go`, `cli.go`).
  The interactive `init` wizard (`internal/cli/interactive/wizard.go`) is linear:
  pick a single provider, no back-nav.
- **Config** (`internal/config/topology.go`): `Topology{Providers map[string]Provider, Routes []Route}`.
  A `Provider` holds at most one credential: a static `APIKey` or a single
  `Credential *CredentialConfig` (`BuildCredential` collapses to one
  `credential.Credential`). A `Route.Chain` is an ordered list of
  `"provider:model"` hops.
- **Credentials** (`internal/credential/store.go`): `credentials.json` maps a
  single key per provider (`map[provider]OAuthRecord`).
- **Routing** (`internal/route/router.go`): `Router.Resolve` returns a
  `core.ResolvedRoute{Hops []core.Hop}` where a `Hop` is `{Provider, Model}`.
- **Proxy** (`internal/proxy/proxy.go`): the only orchestrator. Its attempt loop
  consumes `core.ResolvedRoute.Hops`, consults `core.HealthStore` and
  `core.Selector`, and records via `core.Recorder`. Health/cooldown is keyed by
  *provider*.
- **History** (`internal/history`): `RequestRecord` (in `internal/core/types.go`)
  records `Attempts[]Attempt`, `Provider`, `Usage`, bodies.

Two gaps motivate this change (from the proposal):

1. A provider can only hold **one** credential, so users cannot pool several
   OpenAI keys or rotate several codex OAuth accounts.
2. The router can only express an **ordered linear chain** of hops, not the
   paragraph/pool/fusion ("combo") model proven in the `9router` reference, and
   there is no first-class account-selection strategy or per-account failover.

The proposal converges three capabilities: a full-screen navigable TUI, a
first-class multi-account provider model, and combo routing.

Constraints observed in the codebase that shape this design:

- `proxy` deliberately imports only `internal/core` + stdlib; every concern
  arrives via `Deps` (function values and core interfaces). Any new proxy
  behavior must flow through `core` types/interfaces, **not** direct imports of
## Decisions

### D1 — Account as a first-class runtime type; config additive

Extend `config.Provider` with `Accounts []Account` and `Selection AccountStrategy`
tagged `accounts` / `selection` (both `omitempty`). `Account` carries a `Name`,
a `Type` (`static` | `oauth_refresh`), and the credential fields currently on
`Provider`/`CredentialConfig` (API key, refresh token, client id/secret, token
endpoint, profile, device id). Keep `APIKey`/`Credential` as the implicit
**`default` account** for backward compatibility.

Rationale: mirrors `9router`'s provider-with-accounts shape, keeps config purely
additive, and lets `BuildCredential` become `BuildCredentials(provider) []credential.Credential`
yielding one entry per account. Alternatives considered: a flat `map[account]…`
— rejected because ordering (needed by selection strategies) is lost.

### D2 — Secret store re-keyed to `provider/account`, auto-migrated

Change `credentials.json` keys from `provider` to `provider/account`
(`credentials.json` stays separate from `config.json`; `credential.Store` is the
only component that reads it). Existing single records migrate to `provider/default`.
`Store.Save`/`Delete`/`Get` become account-aware.

Rationale: `credential.Store` already hot-reloads via `fileWatcher`; re-keying
keeps token persistence account-scoped so failover works per account without
touching config. Alternative — storing account secrets inline in config — is
rejected outright by `security.md` (always). Migration is a one-time read+rewrite
on first load.

### D3 — Selection strategy + per-account health in `core`

Add to `internal/core`:

- `AccountStrategy` enum (`round_robin` | `fill_first` | `sticky`) with a pure
  `Select(strategy, accounts, lastUsed, healthy func(account) bool)` helper so
  the strategy logic is unit-testable without I/O.
- A `HopAccount{Provider, Account, Model}` and a `ResolvedRoute` whose hops carry
  an optional `Account`.
- Extend the `HealthStore` interface to be keyed by `provider/account` (keeping
  the provider-level calls working for legacy default-account paths).

Rationale: selection is a pure core concern and must be testable; per-account
cooldown reuses the existing cooldown/backoff machinery by widening its key.
Alternative — performing selection inside `proxy` — is rejected to preserve
proxy's "imports core only" rule.

### D4 — Combo as a config-level logical model expanding to a panel

Add `config.Combo{Name, Members []core.HopAccount, Mode string (ordered|pool|fused), Capabilities []string (hard caps reorder)}`.
Combos live in config and are resolved by an extended `route.Router`:

- Resolver recognizes a combo **name** as a resolvable model and expands it into
  a `ResolvedRoute` whose hops carry ordered/pooled intent.
- Capability reorder: `Capabilities` tiers (`vision` > `pdf` > `audio` > `video`)
  hard-cap which members stay in the panel for a request that needs only a
  subset capability.
- Mode semantics: `ordered` = today's attempt loop; `pool` = treat members as a
  concurrent pool, first success wins; `fused` = parallel fan-out with
  quorum + judge-model synthesis (streaming best-effort).

Rationale: keeps combos declarative in config (hot-reload friendly) and pushes
the expansion through the same resolver the rest of routing uses. The fusion
execution path is the largest slice and is deliberately fenced behind
`core.FusionRunner`, supplied to `proxy.Deps`, so non-fused deploys pay nothing.

  sibling packages.
- Secrets never leave the credential store (`security.md`); config must stay
  additive; legacy fields keep working.
- The CLI is **interactive-first** (`cli-interactivity.md`): with a TTY, prompt;
  non-TTY paths remain scriptable via `--no-interactive`/`--force`.

## Goals / Non-Goals

**Goals:**

- G1 — Model a provider as an **ordered, policy-selected set of accounts**,
  each a named credential (static key or OAuth), with a `selection` strategy
  (`round_robin` | `fill_first` | `sticky`) and per-account cooldown/failover.
- G2 — Express **model combos** as a first-class routing concept: a named
  logical model resolving to an ordered/pooled/fused panel, with capability
  reorder (vision/pdf/audio/video hard-cap tiering).
- G3 — Add a **full-screen navigable TUI** (`tinyroute tui`) and replace the
  linear `init` wizard with a full-screen setup app (non-TTY still scaffolds).
- G4 — Mirror the resulting routing surface through `provider@account:model`
  and combo-name notation end to end (resolution, validation, `models`,
  sessions/status/logs).
- G5 — Keep every existing non-TTY `--no-interactive` path and scriptable table
  output intact and passing.

**Non-Goals:**

- NG1 — A fully feature-parity port of `9router`'s entire provider/fusion
  universe; only the combo *panel* model (ordered/pooled/fused) transfers.
- NG2 — Rewriting the upstream translation layer or the SSE relay semantics.
- NG3 — Changing the persisted `keys.json` (auth) format.
- NG4 — Real-time/non-SQLite history backends.
- NG5 — Auto-discovery/migration of arbitrary `9router` config layouts.
### D5 — Runtime account selection + failover in the attempt loop

The proxy attempt loop, which today tries `Hops` sequentially, gains a per-hop
account dimension: for each `HopAccount` it iterates candidate accounts via
`core.Select` (respecting `round_robin`/`fill_first`/`sticky`), applies
per-account health/cooldown, and on failure pivots to the next account before
the next provider/model hop.

`provider@account:model` pins a single account (no pivoting); `provider@default`
(or bare `provider`) uses the pool via the strategy.

Rationale: reuse of the existing `Deps.Selector`/`HealthStore` seams keeps proxy
from importing sibling packages. Alternative — pre-expanding accounts at
resolution time — is rejected because it would materialize the whole pool on
every request and lose lazy failover.

### D6 — Full-screen TUI via Bubble Tea, additive to existing commands

Add `charmbracelet/bubbletea`, `charmbracelet/bubbles`, `charmbracelet/lipgloss`.
New `tinyroute tui` builds a navigator: sidebar (Providers, API Keys,
Routes/Combos, Sessions, Agents, Status, Live Logs), panes, modals, function-key
shortcuts. `tinyroute init` in a TTY launches a full-screen setup app (multi
provider, back/forward nav); non-TTY falls back to today's scaffold.

Existing per-command tables and `wrapCommand`'s `--no-interactive`/`--force`
ironclad scriptable paths remain untouched. The TUI is a thin presenter over the
same `config`/`credential`/`route`/`history` services the commands already use.

Rationale: TUI should be presentation-only, not a parallel data model. Bubble
Tea is the de-facto Go TUI stack and gives navigation/modals/keybindings for
free. Alternative — extending `pterm` — is rejected: `pterm` is prompt/table
oriented, not a navigator.

### D7 — Agents carry multiple OAuth accounts

`internal/agent` (codex, opencode, kilocode, copilot, hermes, jcode) gains
multi-account rotation: each agent references a provider/account pool and
rotates across accounts (reusing D3's `Select`), e.g. several codex accounts.

Rationale: agents already reconstruct credentials per provider; reusing the core
selector avoids a second selection implementation. Session/status/logs report
the account that served each request (`provider/account` in `core.Attempt` and
`RequestRecord`).

## Risks / Trade-offs

- **[New TUI dependency weight]** → Bubble Tea pulls several transitive deps;
  confined to a new `internal/cli/tui/` package behind the `tui` subcommand, so
  non-TUI builds/scripts are unaffected and `go build` surface stays clean.
- **[credentials.json re-key is breaking (minor)]** → auto-migration on first
  load (D2) plus a `credential.Store` unit test asserting `provider -> provider/default`;
  no manual step.
- **[Fusion is complex and hard to get right]** → fenced behind
  `core.FusionRunner` in `proxy.Deps`; default `proxy` behavior is unchanged for
  `ordered`/`pool`; fusion ships with its own test suite and is documented as
  opt-in per combo.
- **[Selection strategy semantics ambiguity]** → `sticky` needs a client-visible
  anchor; define sticky = session-scoped pin per session fingerprint (already a
  `SessionInputs` concept in `core.ParsedRequest`). Documented in the router
  spec.
- **[Provider with zero accounts / mismatch]** → validation requires ≥1 account
  (or a legacy default) before a hop references the provider; resolver rejects
  `provider@missing` and nonexistent combo names with clear errors.
- **[Backward compat tension in ValidateTopology]** → legacy routes
  (`provider:model`) keep validating as-is; only new fields are enforced when
  present. No existing config is rejected.

## Migration Plan

1. **Data**: add a one-time loader migration in `credential.Store` that re-keys
   `provider` → `provider/default` on first load after upgrade (idempotent;
   `masked` display and `Save`/`Delete` immediately account-aware).
2. **Config**: keep `APIKey`/`Credential` serialized; only append `accounts` /
   `selection` / `combos` when the user adds them. No shrink of existing fields.
3. **Deploy**: additive code paths; the proxy, router, and history all
   degrade to today's behavior when no accounts/combos are declared. Rollback =
   revert the binary; data files are forward- and backward-readable because new
   fields are `omitempty`.
4. **Verification order**: presubmit per change slice (see tasks): (a) accounts +
   selection + health, (b) combos ordered/pool, (c) fusion, (d) TUI + setup app.

## Open Questions

- **Q1** — Should `fill_first` prefer the least-congested *in-flight* account or
  simply the first healthy one by declaration order? Resolve in the router spec
  with a concrete definition.
- **Q2** — Fusion quorum: is a simple majority of member successes sufficient,
  or is judge-model synthesis mandatory before any fusion response is emitted
  in streaming mode? Default: quorum for non-streaming, best-effort synthesis
  for streaming — confirm against 9router behavior during implementation.
- **Q3** — For `sticky` across *multiple* concurrent sessions, the anchor is the
  session fingerprint; but multiple fingerprints could pin to the same account
  and skew it. Decide whether to prefer the least-loaded session-pinned account
  (hybrid round_robin) in the pool spec.
- **Q4** — Where does the combo's capability (vision/pdf/audio/video) originate —
  from the provider preset catalog or inferred from the member models? Default:
  infer from model names via preset catalog with explicit override on the combo.

## Supplement — Slim 7-command base (D8 final)

The command base was slimmed to 7 top-level commands grouped into 6 categories.
The full-screen TUI, `debug`, `validate`, `status`, `compact`, `auth`, and
`provider` top-level commands were all removed. Functionality was either folded
into the kept commands or descoped (TUI).

### Decision D8 (final) — Slim command base with grouped categories

The root `tinyroute` command SHALL register exactly 7 subcommands: `serve`,
`init`, `keys`, `providers`, `combos`, `agent`, `history` — grouped under
Service, Keys, Providers, Combos, Agents, and History categories. Rationale: a
minimal, discoverable command base where every top-level command is a functional
noun (what you manage), and subcommands are the verbs (what you do). The grouped
menu replaces both the flat command list and the earlier full-screen TUI.
Removed commands and where their functionality went:
- `provider` → renamed to `providers` (plural, matches the category)
- `auth` (top-level) → folded into `providers auth` (already existed as a subcommand)
- `debug` → renamed to `history` (the observability command)
- `status` → folded into `history status`
- `compact` → folded into `history compact`
- `validate` → removed; config validation is implicit on `serve` startup and
  via the `ValidateTopology` call in `combos add`
- The `tui` package, `cmdTUI`, and Bubble Tea deps were removed entirely

A new `combos` command (`internal/cli/combos.go`) provides `add`, `list`, and
`remove` subcommands for managing `config.Topology.Combos` — the only
previously-missing CRUD surface for the combos feature.

### Risks / Trade-offs (slim base)

- **[Fewer top-level commands]** → users who memorized `tinyroute debug log` or
  `tinyroute status` need to use `tinyroute history log` / `tinyroute history
  status` instead. Acceptable: the grouping is more discoverable.
- **[No standalone validate]** → `validate` is gone; config errors surface at
  `serve` startup or `combos add` time. Acceptable: both already call
  `ValidateTopology`.
- **[combos add is new]** → no migration needed; it writes to the existing
  `combos` field in `config.json` that the router already reads.



