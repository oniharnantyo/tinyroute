## MODIFIED Requirements

### Requirement: `providers account remove` removes a named account

`tinyroute providers account remove [provider] [name]` SHALL remove the named account
from `Provider.Accounts` and persist via `WriteTopology`. The target SHALL be gathered
interactively when absent. Removing the last account SHALL leave the provider on its
legacy `api_key`/`credential` (if any); it SHALL NOT delete the provider.

Removing an account SHALL additionally downgrade every combo member pinned to it
(`provider@account:model` → `provider:model`), preserving the provider and model. When
the unpinned form is already a member of the same combo, the downgraded entry SHALL be
dropped instead of duplicated. No combo SHALL be removed by an account removal —
downgrade preserves the provider and model of every member, so each combo keeps at
least one member. The command output SHALL name every combo modified as a result.

#### Scenario: Removing an account persists the change

- **WHEN** `tinyroute providers account remove openai secondary` is run
- **THEN** the `secondary` account SHALL be removed from `Provider.Accounts`
- **AND** `WriteTopology` SHALL persist the result

#### Scenario: Removing an account downgrades pinned combo members

- **WHEN** `tinyroute providers account remove glm work` is run and a combo holds
  member `glm@work:glm-4.7`
- **THEN** the member SHALL become `glm:glm-4.7` in the same persisted write
- **AND** the output SHALL name the combo that was modified

#### Scenario: Downgrade that would duplicate drops the pinned member

- **WHEN** a combo holds both `glm:glm-4.7` and `glm@work:glm-4.7` and account `work`
  is removed
- **THEN** the pinned entry SHALL be dropped and no duplicate member SHALL remain

#### Scenario: All-pinned combo survives downgrade with one member

- **WHEN** a combo's members all pin the removed account and downgrade would
  deduplicate them into one
- **THEN** the combo SHALL survive with that single unpinned member and the
  output SHALL name it as modified

#### Scenario: Empty account list is an informational exit

- **WHEN** `providers account remove` targets a provider with no `accounts`
- **THEN** the command SHALL print a clear message and exit without rendering an empty picker
