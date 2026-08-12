# provider-accounts Specification

## Purpose

Defines the multi-account provider model: a provider holds an ordered,
policy-selected set of credentialed accounts, plus runtime account selection and
per-account failover.

## ADDED Requirements

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
