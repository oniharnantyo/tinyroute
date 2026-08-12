# provider-credentials Specification (delta)

## ADDED Requirements

### Requirement: Credential store is keyed by provider and account

The credential custodian SHALL key records by `provider/account` instead of by
provider alone. Each account SHALL have its own credential record so that
refresh and failover are account-scoped. The first load after this change SHALL
migrate any existing provider-keyed record to `provider/default` automatically
and idempotently.

#### Scenario: Existing single record migrates to provider/default

- **WHEN** a credentials file contains a provider-keyed record and is loaded after upgrade
- **THEN** that record SHALL be available under the `provider/default` key
- **AND** the migration SHALL be idempotent across repeated loads

#### Scenario: Multiple accounts have independent records

- **WHEN** a provider has accounts `a1` and `a2`
- **THEN** each SHALL have its own credential record under `provider/a1` and `provider/a2`
- **AND** refreshing one SHALL NOT affect the other

### Requirement: Per-account refresh dedup and cache

Concurrent refresh deduplication SHALL operate per account. A successful refresh
of one account SHALL be cached for that account only, and concurrent waiters for
the same account SHALL collapse to one refresh call.

#### Scenario: Concurrent refresh of the same account collapses

- **WHEN** multiple requests require a refresh of the same `provider/account` simultaneously
- **THEN** exactly one refresh network call SHALL occur
- **AND** all waiters SHALL receive its result

#### Scenario: Different accounts refresh independently

- **WHEN** requests require a refresh of two different accounts of one provider
- **THEN** each account SHALL perform its own refresh as needed
