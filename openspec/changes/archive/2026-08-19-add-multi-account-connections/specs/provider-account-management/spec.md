## MODIFIED Requirements

### Requirement: `providers account add` adds a named account interactively

`tinyroute providers account add [provider] [name]` SHALL append a named account to
`Provider.Accounts`. The provider SHALL be gathered by `Select` from the live
topology when absent and a TTY is attached (single candidate auto-selects;
non-TTY with no provider SHALL yield a clear error naming the value). The account
name SHALL be unique within the provider; a duplicate SHALL be rejected. The
credential type SHALL be `static` (a `Password`-prompted key) or `oauth_refresh`
(delegating to the provider's OAuth flow). For `oauth_refresh`, the delegated flow
SHALL run with the chosen account name as its explicit label, so tokens are stored
under `provider/<name>`. A failed or cancelled OAuth flow SHALL abort the command
with the flow's error and SHALL NOT append a credential-less account entry. The
change SHALL persist via `WriteTopology`. Args/flags (`--account`, `--type`,
`--key`, `--no-interactive`, `--force`) SHALL be honored as an escape-hatch.

#### Scenario: Zero args in a TTY prompts for provider and account name

- **WHEN** `tinyroute providers account add` is run in a terminal with no arguments
- **THEN** the command SHALL offer a `Select` of providers from the topology
- **AND** SHALL prompt for a unique account name and credential type
- **AND** SHALL persist the new account via `WriteTopology`

#### Scenario: Duplicate account name is rejected

- **WHEN** an account name already exists on the target provider
- **THEN** the command SHALL fail with an error naming the provider and the duplicate
- **AND** SHALL NOT modify `config.json`

#### Scenario: Non-TTY with no provider yields a clear error

- **WHEN** `tinyroute providers account add` is run without a TTY and no provider argument
- **THEN** the command SHALL fail with an error naming the provider and how to supply it

#### Scenario: `oauth_refresh` stores tokens under the named account

- **WHEN** `tinyroute providers account add codex team2 --type oauth_refresh`
  completes the delegated OAuth flow
- **THEN** the tokens SHALL be stored under the `codex/team2` key
- **AND** the appended `Accounts[]` entry SHALL carry `type: oauth_refresh`

#### Scenario: OAuth flow failure aborts without appending the account

- **WHEN** the delegated OAuth flow fails or is cancelled
- **THEN** the command SHALL exit non-zero with the flow's error
- **AND** the topology SHALL NOT contain the new account entry
