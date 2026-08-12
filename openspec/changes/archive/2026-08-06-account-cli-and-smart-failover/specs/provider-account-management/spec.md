# provider-account-management Specification

## Purpose

Defines the interactive-first CLI that makes the multi-account provider model
operable without hand-editing `config.json`: adding, listing, removing, testing,
strategy-selecting, and bulk-importing named accounts per provider, plus an
`--account` label on the auth subcommands so credentials land under the intended
`provider/account` key.

## ADDED Requirements

### Requirement: `providers account add` adds a named account interactively

`tinyroute providers account add [provider] [name]` SHALL append a named account to
`Provider.Accounts`. The provider SHALL be gathered by `Select` from the live
topology when absent and a TTY is attached (single candidate auto-selects;
non-TTY with no provider SHALL yield a clear error naming the value). The account
name SHALL be unique within the provider; a duplicate SHALL be rejected. The
credential type SHALL be `static` (a `Password`-prompted key) or `oauth_refresh`
(delegating to the provider's OAuth flow). The change SHALL persist via
`WriteTopology`. Args/flags (`--account`, `--type`, `--key`, `--no-interactive`,
`--force`) SHALL be honored as an escape-hatch.

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

### Requirement: `providers account list` shows accounts with masked credentials and health

`tinyroute providers account list [provider]` SHALL list each account's name, a masked
credential indicator (never plaintext), its current cooldown/health state, and — when
usage tracking is present — a usage snapshot against its quota. The provider SHALL be
prompted when absent and a TTY is attached. An account list SHALL NOT print any stored
access token, refresh token, or `client_secret`.

#### Scenario: Listing shows masked credentials and cooldown state

- **WHEN** `tinyroute providers account list openai` is run for a provider with two accounts
- **THEN** the output SHALL show both account names with masked credential indicators
- **AND** SHALL show each account's cooldown/health state
- **AND** SHALL NOT contain any plaintext secret

#### Scenario: Provider with no accounts reports the legacy default

- **WHEN** a provider declares only `api_key` and no `accounts`
- **THEN** the list SHALL show the implicit `default` account
- **AND** SHALL indicate it is the legacy single-credential path

### Requirement: `providers account remove` removes a named account

`tinyroute providers account remove [provider] [name]` SHALL remove the named account
from `Provider.Accounts` and persist via `WriteTopology`. The target SHALL be gathered
interactively when absent. Removing the last account SHALL leave the provider on its
legacy `api_key`/`credential` (if any); it SHALL NOT delete the provider.

#### Scenario: Removing an account persists the change

- **WHEN** `tinyroute providers account remove openai secondary` is run
- **THEN** the `secondary` account SHALL be removed from `Provider.Accounts`
- **AND** `WriteTopology` SHALL persist the result

#### Scenario: Empty account list is an informational exit

- **WHEN** `providers account remove` targets a provider with no `accounts`
- **THEN** the command SHALL print a clear message and exit without rendering an empty picker

### Requirement: `providers account test` probes a single account

`tinyroute providers account test [provider] [name]` SHALL issue a probe request using
that account's resolved credential, classify the result via `ClassifyFailure`, and
print a human-readable outcome. The target account SHALL be gathered interactively when
absent. A failed probe SHALL NOT penalize the account's runtime health beyond what the
probe itself warrants.

#### Scenario: A healthy account reports success

- **WHEN** `providers account test openai primary` reaches the upstream successfully
- **THEN** the command SHALL report success and the observed latency

#### Scenario: A failing account reports the classified failure

- **WHEN** the probe returns `401`
- **THEN** the command SHALL report an auth failure
- **AND** SHALL NOT trigger chain failover (it is a one-shot probe)

### Requirement: `providers account select` sets the selection strategy

`tinyroute providers account select [provider]` SHALL set `Provider.Selection` by
`Select` over the valid strategies (`round_robin`, `fill_first`, `sticky`,
`sticky_round_robin`) and persist via `WriteTopology`. The strategy MAY also be passed
as an argument or `--strategy` flag.

#### Scenario: Selecting a strategy persists it

- **WHEN** `providers account select openai` picks `round_robin`
- **THEN** `Provider.Selection` SHALL become `round_robin`
- **AND** `WriteTopology` SHALL persist the result

### Requirement: `providers account import` bulk-imports accounts

`tinyroute providers account import [provider]` SHALL accept multiple accounts at once:
`name|key` lines for static keys (with collision-safe naming for unlabeled keys) and a
JSON array for OAuth records. Imported account names SHALL not collide with existing
accounts; collisions SHALL be reported, not silently overwritten.

#### Scenario: Bulk static-key import creates multiple accounts

- **WHEN** `providers account import openai` is given `work|sk-aaa\npersonal|sk-bbb`
- **THEN** two accounts `work` and `personal` SHALL be added
- **AND** `WriteTopology` SHALL persist the result

#### Scenario: Colliding names are reported, not overwritten

- **WHEN** an imported name matches an existing account
- **THEN** the command SHALL report the collision
- **AND** SHALL NOT overwrite the existing account

### Requirement: Account commands are interactive-first and scriptable

Every `providers account` command SHALL follow `.claude/rules/cli-interactivity.md`:
required positional args are optional; missing values are gathered from live state via
`Select`/`MultiSelect`/`Password` when `CanPrompt()`; `--no-interactive`/`--force`
bypass prompts; single-candidate inputs auto-select; empty source lists exit
informationally; pickers draw from real state so selections are valid by construction;
pterm `Filter` is enabled for large lists.

#### Scenario: Flags bypass prompts for scripting

- **WHEN** `providers account add openai work --type static --key sk-aaa --force` is run
- **THEN** the account SHALL be added without any prompt

#### Scenario: Non-interactive mode with missing required values errors clearly

- **WHEN** a required value is absent and `--no-interactive` is set (or no TTY)
- **THEN** the command SHALL fail with an error naming the value and how to supply it
