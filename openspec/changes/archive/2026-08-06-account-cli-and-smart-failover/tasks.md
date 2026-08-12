## 1. Account-management CLI (Phase 1 — highest value, zero engine risk)

- [x] 1.1 Create `internal/cli/account.go` with a `providers account` subgroup (`add`/`list`/`remove`/`test`/`select`/`import`), registered under `cmdProviders` beside `model` in `internal/cli/commands.go`
- [x] 1.2 `account add [provider] [name]`: interactive-first provider `Select` from topology, unique-name validation, cred type `Select` (`static` → `Password`; `oauth_refresh` → delegate to existing OAuth flow), append to `Provider.Accounts`, `WriteTopology`; honor `--account`/`--type`/`--key`/`--no-interactive`/`--force`
- [x] 1.3 `account list [provider]`: name + masked cred (`OAuthRecord.Masked`) + `HealthStore.ActiveCooldowns` + (Phase 4) usage snapshot; never print plaintext
- [x] 1.4 `account remove [provider] [name]` + `account test [provider] [name]` (probe via `ClassifyFailure`, no chain failover); empty-list informational exit
- [x] 1.5 `account select [provider]`: `Select` over `round_robin`/`fill_first`/`sticky`/`sticky_round_robin` → `Provider.Selection`, `WriteTopology`; honor `--strategy`
- [x] 1.6 `account import [provider]`: `name|key` lines (collision-safe naming for unlabeled) + JSON array for OAuth; report collisions, never overwrite
- [x] 1.7 Thread `--account` through `cmdAuthSet` (write into `Provider.Accounts[]`, not `p.APIKey`) in `internal/cli/commands.go` and `cmdAuthLogin`/`cmdAuthImport` (`OAuthRecord{Account}`) in `internal/cli/auth.go`; omitting `--account` preserves legacy behavior

## 2. Per-model cooldown + resets_at-aware cooldown (Phase 2)

- [x] 2.1 `internal/route/health.go`: add `AvailableModel(key, model string) bool` (model key then account-wide fallback) and `PenalizeModel(key, model string, d time.Duration)` using composite keys; extend `stateEntry`/`Save`/`Load` with optional `Model`; keep `Available`/`Penalize` working unchanged
- [x] 2.2 `internal/core/reset_parser.go` (interface) + `reset_parser_impl.go` (`StandardResetParser`: HTTP `Retry-After`, OpenAI/Anthropic rate-limit JSON, Codex `usage_limit_reached`); fail-open, cap at existing max
- [x] 2.3 `internal/proxy/proxy.go`: build the `available` closure closing over the hop model → `AvailableModel`; call `ResetParser.Duration(resp, body, fc)` in the hop loop and pass to `PenalizeModel`, falling back to `Cooldown429`/`Cooldown5xx`

## 3. Sticky round-robin (Phase 2)

- [x] 3.1 `internal/core/account.go`: add `StrategyStickyRoundRobin`; add `Affinity` interface (`Count`/`Touch`/`Reset`) and `SelectAccountsAffinity(accounts, strategy, available, counter, affinity) []string`; leave `SelectAccounts` untouched
- [x] 3.2 `internal/route/health.go`: add a non-persisted affinity map implementing `Affinity`
- [x] 3.3 `internal/config/topology.go`: add `Provider.StickyLimit int` (default `3` when strategy is `sticky_round_robin`); validation
- [x] 3.4 `internal/proxy/proxy.go`: route `sticky_round_robin` to `SelectAccountsAffinity`, wiring the affinity store and limit

## 4. Usage tracking + quota gate (Phase 3)

- [x] 4.1 `internal/core/usage.go` (types): `QuotaConfig{Window time.Duration; Tokens int64; Requests int}`; `UsageSnapshot{Window, UsedTokens, UsedRequests, LimitTokens, LimitRequests, ResetAt}`
- [x] 4.2 `internal/core/usage_store.go` (impl): `UsageStore` mutex-guarded rolling-window buckets keyed `"provider/account"`; `Record(key, Usage)`, `Snapshot(key, QuotaConfig) UsageSnapshot`, `Exhausted(key, QuotaConfig) bool`; nil-safe (no persistence)
- [x] 4.3 `internal/config/topology.go`: add `Account.Quota *QuotaConfig` (nil = unlimited); optional `Provider.Quota` default
- [x] 4.4 `internal/proxy/proxy.go`: add `!usage.Exhausted(hopKey, quota)` to the `available` closure; record `usage.Record(winningProvider, *finalUsage)` after success; add `Usage *core.UsageStore` to `ProviderInfo`/`Deps` (nil disables)

## 5. Tiered budget fallback (Phase 3)

- [x] 5.1 Document tier chains as multi-hop `Route.Chain` (subscription → cheap → free) with quota-gated accounts; add an example to `docs/ARCHITECTURE.md`
- [x] 5.2 (Optional polish) `tinyroute providers account import` / `combos add` helper to scaffold a tier chain; defer if low value

## 6. Tests & verification

- [x] 6.1 `internal/cli/account_test.go`: add/list/remove/select/import — single/multiple/zero accounts, duplicate-name rejection, `--account` auth wiring, masked output, non-TTY errors, flag bypass
- [x] 6.2 `internal/route/health_test.go`: per-model isolation, account-wide fallback, affinity pin/rotate/reset; `internal/core/reset_parser_impl_test.go`: Retry-After/JSON/Codex fixtures + cap + fail-open
- [x] 6.3 `internal/core/usage_store_test.go`: window rollover, exhaustion, nil-quota unlimited; proxy test: exhausted account skipped pre-request, chain descends, nil store no-op
- [x] 6.4 Regression: legacy single-`APIKey` provider, `"default"` account, non-combo routes unchanged; grep all `Available(`/`Penalize(`/`SelectAccounts(` callers
- [x] 6.5 `gofmt -w .`; `go build ./...` clean; `go test ./...` green; ≥80% coverage on new files; `openspec validate account-cli-and-smart-failover` and `openspec status --change account-cli-and-smart-failover` complete
