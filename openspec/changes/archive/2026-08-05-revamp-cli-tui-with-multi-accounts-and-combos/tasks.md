## 1. Config schema — accounts, selection, combos

- [x] 1.1 Add `Account` (Name, Type, APIKey, Credential fields) and `AccountStrategy` types; add `Accounts []Account` and `Selection` to `config.Provider` (JSON tags `accounts`, `selection`, omitempty) in `internal/config/topology.go`
- [x] 1.2 Add `Combo` (Name, Members, Mode, Capabilities) and `Combos` to `Topology`; parse members as `provider[(@account)]:model` / combo names in `ParseTopology`
- [x] 1.3 Keep legacy `APIKey`/`Credential` as implicit `default` account; extend `BuildCredential` → account-aware `BuildCredentials(provider) []credential.Credential`
- [x] 1.4 Extend `ValidateTopology` to reject duplicate account names, unknown selection strategy, combos with unknown providers/accounts/modes, and cross-dialect combo members without a translator; legacy routes still validate as before

## 2. Credential store — provider/account re-keying + migration

- [x] 2.1 Re-key `credential.Store` records to `provider/account`; update `Save`/`Delete`/`Get`/`List` signatures to account-aware
- [x] 2.2 Add one-time idempotent migration on first load: provider-keyed records → `provider/default`
- [x] 2.3 Keep `Masked()` and list rendering account-aware and secret-safe (no plaintext leakage)

## 3. Core — selection strategy and per-account health

- [x] 3.1 Add `AccountStrategy` enum and pure `core.Select` helper (round_robin/fill_first/sticky) with unit tests in `internal/core`
- [x] 3.2 Extend `core.Hop`/`ResolvedRoute` to carry an optional account (`HopAccount{Provider, Account, Model}`) and combo/pool intent without breaking existing callers
- [x] 3.3 Widen `core.HealthStore` to be keyed by `provider/account` while keeping provider-level legacy calls working (default account)

## 4. Router — account and combo resolution

- [x] 4.1 Parse `provider@account:model` in `route.Router.Resolve` (pinned account) and `provider@default:model` (pool via strategy)
- [x] 4.2 Resolve named combos into expanded member hops with mode + capability intent
- [x] 4.3 Apply capability reorder (vision > pdf > audio > video hard-cap tiering) to combo panels
- [x] 4.4 List combos and account-pinned identifiers in `Router.Models` for discovery; agree with `Resolve`
- [x] 4.5 Add router unit tests: account-pinned, combo expansion, capability tiering, unknown account/combo errors

## 5. Proxy — account selection and failover in the attempt loop

- [x] 5.1 In `internal/proxy`, per-hop account iteration using `core.Select`, respecting strategy and per-account health, pivoting to next account on retryable failure
- [x] 5.2 Honor pinned (`provider@account`) vs pool (`provider@default`) hops; exhaust all accounts before moving to the next hop
- [x] 5.3 Keep `proxy` imports restricted to `core` + stdlib; wire selection/health via `Deps` seams
- [x] 5.4 Add proxy tests: failover across accounts, pool exhaustion, pinned-account no-pivot, round_robin/fill_first/sticky behavior

## 6. Fusion execution

- [x] 6.1 Define `core.FusionRunner` interface and a `pool`/`fused` execution path wired via `proxy.Deps` (opt-in per combo, non-fused paths unchanged)
- [x] 6.2 Implement parallel fan-out for `pool` (first success wins) and `fused` (parallel fan-out + quorum + judge-model synthesis; best-effort streaming)
- [x] 6.3 Add fusion tests: quorum failure, streaming best-effort, `ordered`/`pool` pay no fusion cost

## 7. History — serving account

- [x] 7.1 Add serving `provider/account` to `core.Attempt`/`RequestRecord` and persist in SQLite schema (new column, additive)
- [x] 7.2 Surface serving account in `sessions`/replay/log views; support query filter by provider/account

## 8. Agents — multi-account rotation

- [x] 8.1 Allow `internal/agent` adapters to reference a provider/account pool and rotate via `core.Select` (e.g. multiple codex accounts)
- [x] 8.2 Surface the serving account in agent status/log views; add rotation/failover tests

## 9. Slim command base (7 top-level commands: serve, init, keys, providers, combos, agent, history)

- [x] 9.1 Rename `provider` → `providers`; fold top-level `auth` into `providers auth` subcommand; remove standalone `auth` and `provider` registrations
- [x] 9.2 Rename `debug` → `history`; fold `status` and `compact` into `history status` / `history compact` subcommands; remove standalone `debug`, `status`, `compact` registrations
- [x] 9.3 Create new `combos` command (`internal/cli/combos.go`) with `add`, `list`, `remove` subcommands that manage `config.Topology.Combos` via `ParseTopology`/`ValidateTopology`/`WriteTopology`
- [x] 9.4 Remove `validate` from the root registration (config validation is implicit on `serve` startup and via `combos add` / `WriteTopology` paths)
- [x] 9.5 Update `cli.go` categories to: Service, Keys, Providers, Combos, Agents, History; register only the 7 kept commands
- [x] 9.6 Verify `tinyroute --help` renders 6 grouped sections with exactly 7 commands; `--no-interactive`/`--force` scriptable paths intact

## 10. Init — interactive wizard (reverted to pterm-based)

- [x] 10.1 `tinyroute init` in a TTY runs `interactive.RunInitWizard` (multi-step, back navigation, transactional save); non-TTY falls back to scaffold
- [x] 10.2 Verify no `tui` import or `RunWizard` reference remains in `internal/cli`

## 11. Tests

- [x] 11.1 Config: accounts/selection/combos parse + validation cases (duplicate account, unknown mode/provider/account)
- [x] 11.2 Credential: `provider→provider/default` migration idempotency; per-account independence; account-scoped refresh dedup
- [x] 11.3 Core: `Select` strategy unit tests; per-account HealthStore
- [x] 11.4 Router: account-pinned, combo expansion, capability reorder, discovery agreement
- [x] 11.5 Proxy: account failover, pool exhaustion, pinned no-pivot; fusion quorum/streaming
- [x] 11.6 History: serving-account persistence, query filter, view surfacing
- [x] 11.7 Agent: multi-account rotation + failover
- [x] 11.8 Verify `go test ./internal/cli/...` green after TUI removal and category additions

## 12. Verification

- [x] 12.1 `gofmt -w .`
- [x] 12.2 `go build ./...` clean (Bubble Tea deps removed)
- [x] 12.3 `go test ./...` green across all touched packages

## 13. (Removed — functional TUI panes descoped)

- [x] 13.1 ~~Functional TUI panes~~ — descoped: the full-screen TUI was removed in favor of the grouped command menu (§9). The existing CLI commands already provide the read/write operations the panes would have duplicated. See `cli-tui-navigator` spec REMOVED requirements for rationale and migration.

- [x] 12.4 Legacy single-credential provider and non-combo routes continue to work unchanged (no config rejected); `openspec validate` and status fully complete
