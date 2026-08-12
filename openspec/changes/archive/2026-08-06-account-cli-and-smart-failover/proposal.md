## Why

The multi-account engine already exists and is archived in
`2026-08-05-revamp-cli-tui-with-multi-accounts-and-combos`: `Provider.Accounts`,
`Selection` (`round_robin`/`fill_first`/`sticky`), per-account `HealthStore`
cooldown/failover, `provider@account:model` routing, combos, and `provider/account`
history attribution are all shipped. That change **descoped the account-management
UI** (its §13), asserting "the existing CLI commands already provide the read/write
operations the panes would have duplicated." They do not: there is no `account`
subgroup, `providers auth set` still writes only the single `Provider.APIKey`, and
`providers auth login`/`import` key credentials as `"provider/default"` with no
account label. The result is an engine that is unreachable except by hand-editing
`config.json` — exactly the gap users hit when asking to "support multi account or
api key on providers."

Separately, the `9router` reference (in `9router/`) demonstrates three selection
behaviors tinyroute lacks: per-model cooldown isolation, cooldown durations read
from the upstream's real reset time, and sticky round-robin with a consecutive-use
limit; plus the headline "use every bit before reset" via per-account quota
tracking and tiered (subscription→cheap→free) budget fallback.

## What Changes

- **Account-management CLI (fills the gap).** A new `providers account` subgroup
  (`add`/`list`/`remove`/`test`/`select`/`import`), interactive-first per
  `.claude/rules/cli-interactivity.md`, plus an `--account` flag on
  `providers auth set`/`login`/`import` so credentials land under `provider/NAME`
  instead of `default`. Zero engine changes; pure CLI layer.
- **Smart-failover refinements.** Per-model cooldown keys; `resets_at`/`Retry-After`-
  aware cooldown durations; a `sticky_round_robin` strategy with a consecutive-use
  limit.
- **Budget-aware selection + tiered fallback.** An in-memory `UsageStore` accumulates
  per-account-per-window usage from `RequestRecord.Usage`; exhausted accounts are
  skipped pre-request. Tiers are expressed as multi-hop `Route.Chain` whose accounts
  are quota-gated, so the chain descends subscription→cheap→free automatically — no
  new top-level concept.

## Capabilities

### New Capabilities
- `provider-account-management`: interactive-first CLI to add, list, remove, test,
  select strategy for, and bulk-import accounts per provider; `--account` on the
  auth subcommands.

### Modified Capabilities
- `provider-accounts`: `sticky_round_robin` strategy; per-model cooldown isolation;
  upstream-reset-aware cooldown; quota-aware account selection; tiered budget
  fallback via chain + quota gate.
- `provider-credentials`: `--account` flag on `providers auth set`/`login`/`import`
  writes the `provider/account` record (legacy single-key path unchanged).

## Impact

- **Code**: `internal/cli/account.go` (new) + registration in `internal/cli/commands.go`;
  `--account` threading in `internal/cli/commands.go` (`cmdAuthSet`) and
  `internal/cli/auth.go` (`cmdAuthLogin`, `cmdAuthImport`); `internal/route/health.go`
  (model-aware + affinity methods); `internal/core/account.go`
  (`StrategyStickyRoundRobin`, `SelectAccountsAffinity`, `Affinity` interface);
  `internal/core/reset_parser.go` + `reset_parser_impl.go` (new);
  `internal/core/usage.go` + `usage_store.go` (new); `internal/config/topology.go`
  (`Account.Quota`, `Provider.StickyLimit`); `internal/proxy/proxy.go` (model-aware
  closure, reset parsing, quota gate, usage recording, affinity wiring).
- **Dependencies**: none (reuses existing `urfave/cli/v3`, `pterm`, stdlib).
- **Data**: `config.json` (additive `accounts[].quota`, `sticky_limit`);
  `state.json` (cooldown entries gain optional model; affinity is not persisted);
  no `credentials.json` change (already `provider/account`).
- **BREAKING**: none. Legacy single-`api_key` providers and `"default"` accounts
  behave exactly as before; `Quota`/`StickyLimit` default to off.