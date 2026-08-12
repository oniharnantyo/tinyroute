## Context

The multi-account engine landed in the archived change
`2026-08-05-revamp-cli-tui-with-multi-accounts-and-combos`. Verified current state:

- `internal/config/topology.go`: `Provider{Accounts []Account, Selection,
  Cooldown429, Cooldown5xx}`, `Account{Name, Type, APIKey, Credential}`.
- `internal/core/account.go`: `SelectAccounts(accounts, strategy, available
  func(string) bool, counter *uint64) []string`; `HopAccount{Provider, Account,
  Model}`; `Hop.HopKey()` → `"provider/account"`.
- `internal/route/health.go`: `HealthStore` keyed `"provider/account"` with
  `Available`, `Penalize(key, duration)` (**already does strike escalation: doubles,
  capped 5min, reset via `ClearStrikes`**), `Save`/`Load` to `state.json` (atomic
  tmp+rename, 0600).
- `internal/proxy/proxy.go`: per-hop account iteration via `SelectAccounts` with an
  `available` closure `deps.Health.Available(hop.Provider+"/"+acc)`; on failure
  `ClassifyFailure` → `Penalize`; records `winningProvider` (`"provider/account"`)
  and `Usage` via `recordOutcome` off the hot path.
- `internal/credential/store.go`: `OAuthRecord{Account}`, keyed `"provider/account"`;
  `Provider.BuildAccountCredential(provider, account, store)` already falls back to
  single `APIKey` when no account matches.
- `internal/cli/commands.go`: `providers` has `add`/`auth`/`model` only;
  `cmdAuthSet` writes `p.APIKey`; `providers model …` is the interactive-first
  reference. No `account` subgroup; `Selection` is config-file-only.

## Goals / Non-Goals

**Goals**
- Make the existing multi-account engine fully operable from the CLI, interactive-first.
- Port the three `9router` selection refinements that tinyroute lacks.
- Add per-account quota tracking and tiered budget fallback without a parallel
  routing structure.

**Non-Goals**
- Re-implement engine multi-account, selection, combos, or per-account failover
  (already shipped).
- A full-screen TUI (already descoped in the prior change; the grouped command menu
  is the UI).
- Cross-instance shared quota state (tinyroute is a single-process local gateway;
  usage is in-memory and resets on restart).
- Re-adding exponential backoff levels — strike escalation already exists in `Penalize`.

## Decisions

**1. Per-model cooldown via composite key, no `SelectAccounts` signature change.**
- **Decision:** Reuse the existing `cooldowns map[string]time.Time` with composite
  keys `"provider/account/model"`. Add `HealthStore.AvailableModel(key, model string)
  bool` (checks the model key, then falls back to the account-wide key) and
  `PenalizeModel(key, model string, d time.Duration)`. The proxy builds the
  `available` closure closing over the hop's model, so `SelectAccounts` is untouched.
- **Rationale:** The `available` predicate is already a caller-built closure that
  knows the model in scope. Per-model isolation is a placement change at the call
  site, not a new abstraction.

**2. `resets_at`-aware cooldown reuses `Penalize`.**
- **Decision:** A `ResetParser` interface (`Duration(resp, body, class) time.Duration`)
  with a `StandardResetParser` impl (HTTP `Retry-After`, OpenAI/Anthropic rate-limit
  JSON, Codex `usage_limit_reached`). The proxy calls it in the hop loop where the
  response is in hand and passes the result to the existing `Penalize`/`PenalizeModel`,
  falling back to configured `Cooldown429`/`Cooldown5xx` on parse miss. Capped by the
  existing 5min ceiling.
- **Rationale:** `Penalize` already accepts a duration and already escalates via
  strikes. The only new logic is *computing* the duration from the upstream signal.

**3. Sticky round-robin as a new strategy + ephemeral affinity, not a counter change.**
- **Decision:** Add `StrategyStickyRoundRobin` and `Provider.StickyLimit` (default 3).
  Add an `Affinity` interface (`Count`/`Touch`/`Reset`) backed by a non-persisted map
  on `HealthStore` (rotation affinity need not survive restart, unlike cooldowns), and
  a new `SelectAccountsAffinity(accounts, strategy, available, counter, affinity)`
  function. The existing `SelectAccounts` is left untouched.
- **Rationale:** `SelectAccounts` is a pure function over a single atomic counter;
  consecutive-use pinning needs richer per-account state. A sibling function avoids
  perturbing the proven path.

**4. Usage tracking in-memory, fed from existing `RequestRecord.Usage`.**
- **Decision:** A `UsageStore` (mutex-guarded, mirroring `HealthStore`'s style —
  explicitly **not** SQLite-per-request, the `9router` anti-pattern) keeps rolling
  window buckets per `"provider/account"`, accumulated from `Usage` after a successful
  hop. Limits come from a new `Account.Quota *QuotaConfig{Window, Tokens, Requests}`
  (nil = unlimited).
- **Rationale:** `Usage` is already recorded per request. Accumulation is a
  write-after-success off the hot path; reads are an in-memory snapshot.

**5. Quota + tiers compose via the `available` predicate and `Route.Chain` — no new type.**
- **Decision:** The proxy's `available` closure gains `!usage.Exhausted(hopKey,
  quota)`. An over-budget account is skipped *pre-request*, so an ordered `Route.Chain`
  (subscription→cheap→free) descends to the next hop with no error. Quota is a
  pre-filter; HTTP-error failover is the existing path; they do not overlap.
- **Rationale:** tinyroute already has ordered failover via `Chain` and a single
  choke point (`available`) for skipping unusable accounts. A parallel `Tiers`
  concept would duplicate both.

**6. Account CLI follows the `providers model` reference.**
- **Decision:** A new `internal/cli/account.go` registers the `account` subgroup under
  `providers` beside `model`, using the `interactive.*` primitives and honoring
  `CanPrompt()`, `--no-interactive`/`--force`, options-from-live-state, single-candidate
  auto-select, empty-list informational exit, and pterm `Filter`.
- **Rationale:** Reuses the proven interactive-first pattern (`.claude/rules/cli-interactivity.md`)
  and keeps the change shallow.

## Risks / Trade-offs

- **Risk: command-tree drift.** The prior change renamed `provider`→`providers` and
  folded `auth` into `providers auth`. The new subgroup must register under the
  current tree (`providers account`), and `--account` must thread through `providers
  auth set/login/import` as they exist today.
  - *Mitigation:* grep `cmdProviders`/`cmdAuth*` registrations before editing; spec
    scenarios pin the exact user-facing paths.
- **Risk: `HealthStore` call-site breadth.** Model-aware methods + affinity touch the
  most-shared type. A missed caller regresses the single-account path.
  - *Mitigation:* grep all `Available(`/`Penalize(`/`SelectAccounts(` callers; keep
    legacy `Available`/`Penalize` working unchanged; TDD the single-account regression.
- **Risk: ephemeral usage resets on restart.** Budget steering restarts from zero
  each launch.
  - *Trade-off:* acceptable for a local single-process gateway; documented as a
    non-goal. `state.json` persists cooldowns (which carry the safety), not usage.
- **Risk: per-provider `resets_at` body variance.**
  - *Mitigation:* start with the three documented shapes; fail-open to configured
    cooldowns; never exceed the cap; never block on a parse error.
