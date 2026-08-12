# provider-accounts Specification

## ADDED Requirements

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
