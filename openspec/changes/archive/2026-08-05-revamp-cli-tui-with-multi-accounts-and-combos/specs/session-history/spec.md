# session-history Specification (delta)

## ADDED Requirements

### Requirement: Records carry the serving account

Each history record SHALL carry the serving `provider/account` in addition to
the serving provider. Replay and session views SHALL surface the account that
served each request so multi-account and combo routing are observable.

#### Scenario: Serving account is recorded for a successful hop

- **WHEN** a request is served successfully by account `a2` of provider `p`
- **THEN** the history record SHALL carry `p/a2` as the serving provider/account

#### Scenario: Serving account is empty when no provider served

- **WHEN** a request fails before any provider/account serves it
- **THEN** the history record's serving provider/account field SHALL be empty

### Requirement: History is queryable by serving account

The history query layer SHALL support filtering by serving `provider/account`
alongside the provider filter, so multi-account traffic can be inspected per
account.

#### Scenario: Filter by account returns only that account's records

- **WHEN** history is queried with a provider/account filter
- **THEN** every returned record SHALL have that provider/account as its serving account
