# provider-accounts Specification

## Purpose

Defines the multi-account provider model: a provider holds an ordered,
policy-selected set of credentialed accounts, plus runtime account selection and
per-account failover.

## Requirements

### Requirement: A provider holds an ordered, policy-selected set of accounts

A provider SHALL be able to declare an ordered list of `accounts`, each a named
credential (`static` key or `oauth_refresh`), plus a `selection` strategy of
`round_robin`, `fill_first`, or `sticky`. When no `accounts` are declared, the
legacy single `api_key` / `credential` SHALL remain valid as the implicit
`default` account and SHALL behave exactly as before. An account name SHALL be
unique within its provider.

#### Scenario: Provider with no accounts uses the legacy default account

- **WHEN** a provider declares only `api_key` (or a single `credential` block) and no `accounts`
- **THEN** the provider SHALL authenticate as the implicit `default` account
- **AND** SHALL behave identically to the pre-change single-credential provider

#### Scenario: Provider with multiple named accounts

- **WHEN** a provider declares two static-key accounts named `primary` and `secondary` plus `selection: "round_robin"`
- **THEN** the provider SHALL expose both accounts with their declared order and selection strategy

#### Scenario: Duplicate account name is rejected

- **WHEN** a provider declares two accounts with the same name
- **THEN** `ValidateTopology` SHALL return an error naming the provider and the duplicated account

### Requirement: Account selection strategies

The system SHALL select the account for each hop according to the provider's
`selection` strategy. `round_robin` SHALL advance to the next healthy account
for each successive request. `fill_first` SHALL use the first healthy account by
declaration order, moving down the list only when earlier accounts are
unhealthy. `sticky` SHALL pin a session to one account based on its session
fingerprint for the duration of that session.

#### Scenario: Round-robin advances across requests

- **WHEN** a provider with two healthy accounts uses `round_robin`
- **THEN** successive requests are served by different accounts in declaration order, wrapping around

#### Scenario: Fill-first prefers the earliest healthy account

- **WHEN** a provider uses `fill_first` and its first account is healthy
- **THEN** every request is served by the first account

#### Scenario: Fill-first skips a cooled-down account

- **WHEN** a provider uses `fill_first` and its first account is on cooldown
- **THEN** the request is served by the next healthy account
- **AND** the cooled-down account is used again once its cooldown expires

#### Scenario: Sticky pins a session to one account

- **WHEN** a provider uses `sticky` and a session fingerprint is known
- **THEN** all requests in that session are served by the same account

### Requirement: Per-account health and failover

The system SHALL track cooldown/backoff per provider-account pair, not per
provider alone. When a request fails on an account with a retryable failure
class, the attempt loop SHALL pivot to the next account for that hop before
moving to the next provider/model hop. Cooldown and backoff SHALL apply to the
failing account only.

#### Scenario: A failing account triggers failover to the next account

- **WHEN** a hop references a provider with accounts `a1`, `a2` and account `a1` returns a retryable failure (e.g. `429` or `5xx`)
- **THEN** the attempt loop SHALL retry the same hop on account `a2`
- **AND** account `a1` SHALL be cooled down, but account `a2` SHALL remain usable

#### Scenario: All accounts of a hop are exhausted

- **WHEN** every account of a hop fails with a retryable failure
- **THEN** the attempt loop SHALL move to the next hop in the route chain
- **AND** the outcome SHALL reflect the chain/hop exhaustion

### Requirement: Routeable account notation

The system SHALL support `provider@account:model` to pin a single account, and
`provider@default:model` (or bare `provider:model`) to use the provider's
account pool via its selection strategy. A reference to a nonexistent account or
provider SHALL be a resolution error.

#### Scenario: Pinned account is used without pivoting

- **WHEN** a hop is `provider@account:model`
- **THEN** only that account serves the hop
- **AND** no pivoting to other accounts SHALL occur

#### Scenario: Pool notation uses the selection strategy

- **WHEN** a hop is `provider@default:model` and the provider has multiple accounts
- **THEN** the account is chosen by the provider's selection strategy
- **AND** failover pivots across the pool

#### Scenario: Unknown account is a resolution error

- **WHEN** a hop references `provider@nope` and `nope` is not a declared account
- **THEN** route resolution SHALL fail with an error naming the missing account

### Requirement: Sticky round-robin selection

The system SHALL support a `sticky_round_robin` selection strategy that pins a
provider to one account for up to `Provider.StickyLimit` (default `3`) consecutive
requests, then rotates to the next healthy account. Pinning SHALL reset when the
pinned account becomes unavailable. When `StickyLimit` is zero or unset, the strategy
SHALL default to a limit of `3`. The affinity state SHALL be tracked per
`provider/account` and SHALL NOT be persisted across restarts (rotation affinity is
ephemeral; cooldowns remain the persisted safety).

#### Scenario: Sticky round-robin pins then rotates

- **WHEN** a provider with two healthy accounts uses `sticky_round_robin` with limit `2`
- **AND** three successive requests arrive
- **THEN** the first two requests SHALL be served by the same account
- **AND** the third request SHALL be served by the next account

#### Scenario: An unavailable pinned account resets the affinity

- **WHEN** the pinned account is cooled down mid-window
- **THEN** the next request SHALL be served by another healthy account
- **AND** the cooled-down account SHALL resume as a candidate once its cooldown expires

### Requirement: Per-model cooldown isolation

The system SHALL track cooldowns per `provider/account/model`, with an account-wide
fallback at `provider/account`. A retryable failure on one model for an account SHALL
NOT make that account unavailable for a different model. When a model-specific entry
is absent, the system SHALL consult the account-wide entry.

#### Scenario: A 429 on one model does not block another model on the same account

- **WHEN** account `a1` returns `429` for `gpt-4`
- **THEN** `a1` SHALL be skipped for `gpt-4` during its cooldown
- **AND** `a1` SHALL remain available for `gpt-4o-mini` on the next request

#### Scenario: Account-wide cooldown still applies when no model-specific entry exists

- **WHEN** an account is cooled down account-wide and a model-specific entry is absent
- **THEN** that account SHALL be unavailable for every model until the cooldown expires

### Requirement: Upstream-reset-aware cooldown duration

When an upstream rate-limit or quota error includes a reset signal — HTTP `Retry-After`,
the rate-limit JSON `reset`/`resets_at`, or a provider-specific body such as Codex
`usage_limit_reached` — the cooldown duration SHALL be derived from that signal (time
until reset), capped at the existing maximum cooldown. When no signal is present, the
configured `Cooldown429`/`Cooldown5xx` SHALL apply as today. Parsing SHALL fail open:
a malformed body SHALL NOT lengthen a cooldown beyond the configured default and SHALL
NOT block the request path.

#### Scenario: A Retry-After header sets the cooldown

- **WHEN** an upstream returns `429` with `Retry-After: 120`
- **THEN** the account SHALL be cooled down for approximately 120 seconds (subject to the cap)
- **AND** SHALL NOT use the default `Cooldown429` duration

#### Scenario: Missing reset signal falls back to the configured cooldown

- **WHEN** an upstream returns `429` with no reset signal
- **THEN** the account SHALL be cooled down for the configured `Cooldown429` duration

### Requirement: Quota-aware account selection

The system SHALL track per-account usage within a rolling time window and SHALL skip,
pre-request, any account whose configured `Quota` for the current window is exhausted.
`Quota` SHALL be optional: an account without a `Quota` is unlimited and behaves as
before. Usage SHALL be accumulated from each successful request's token usage off the
critical path.

#### Scenario: An exhausted account is skipped before the request

- **WHEN** account `a1` has a daily token `Quota` that has been reached
- **THEN** the selection SHALL skip `a1` without attempting it
- **AND** SHALL proceed to the next healthy, non-exhausted account

#### Scenario: An account without a quota is never skipped for budget

- **WHEN** an account declares no `Quota`
- **THEN** usage tracking SHALL NOT influence its selection regardless of accumulated usage

### Requirement: Tiered budget fallback via ordered chain

A budget tier chain SHALL be expressed as an ordered `Route.Chain` of hops
(e.g. subscription → cheap → free). Because quota-exhausted accounts are skipped
pre-request by the same availability predicate used for cooldowns, exhausting a tier's
accounts SHALL cause the chain to descend to the next hop without surfacing an error.
Tiered fallback SHALL compose with existing HTTP-error failover; the two SHALL NOT
require separate configuration structures.

#### Scenario: Tier descent on quota exhaustion

- **WHEN** the first hop's accounts are all quota-exhausted for the current window
- **THEN** the request SHALL be served by the next hop in the chain
- **AND** no error SHALL be surfaced to the client for the skipped tier

#### Scenario: HTTP failure still triggers hop failover within a tier

- **WHEN** a hop's serving account returns `5xx`
- **THEN** the existing retry/failover path SHALL apply
- **AND** quota gating SHALL continue to skip exhausted accounts in the same loop
