# agent-install Specification (delta)

## ADDED Requirements

### Requirement: An agent may reference multiple OAuth accounts

An agent (for example codex) SHALL be able to hold multiple credentialed
accounts, managed through the provider UI, and SHALL rotate across them using
the provider's account selection strategy. Credential resolution for an agent
SHALL follow the account pool the agent references.

#### Scenario: Agent rotates across several accounts

- **WHEN** an agent references a provider/account pool with two accounts and a `round_robin` strategy
- **THEN** successive agent requests SHALL be served by different accounts in declaration order

#### Scenario: Agent account failover on failure

- **WHEN** the currently selected agent account returns a retryable failure
- **THEN** the agent SHALL pivot to the next account in its pool
- **AND** the failing account SHALL be cooled down

### Requirement: Agent status surfaces the account that served each request

Agent status and session/log views SHALL report which `provider/account` served
each request so multi-account rotation is observable.

#### Scenario: Status shows the serving account

- **WHEN** an agent request was served by account `a1` of provider `codex`
- **THEN** the corresponding status/log entry SHALL identify `codex/a1` as the serving account
