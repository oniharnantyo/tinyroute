## Purpose

Guided, interactive-first creation and management of model combos on the CLI:
the step-by-step wizard that builds a multi-model combo, the ordered
member selection flow (including members pinned to a specific provider
account), write-time member validation, and the behavior when interactivity
or topology state is unavailable.

## ADDED Requirements

### Requirement: `combos add` runs as a step-by-step wizard

When run in an interactive terminal with no arguments, `tinyroute combos add`
SHALL present a sequential wizard with five steps: name, members in priority
order, execution mode, optional capabilities, and a review/confirm summary.
Each step SHALL present exactly one decision. The command SHALL NOT require
typed positional arguments or flags in a TTY.

#### Scenario: Zero-arg run in a TTY starts the wizard

- **WHEN** `tinyroute combos add` is run without arguments and stdin is a TTY
- **THEN** the wizard SHALL start at the name step and proceed one step at a
  time until confirmation or cancellation

#### Scenario: Wizard collects a complete combo

- **WHEN** the user completes all five steps and confirms the review summary
- **THEN** the combo SHALL be written to configuration with the entered name,
  members in picked order, selected mode, and optional capabilities
- **AND** the CLI SHALL print how clients use it (request the combo name as
  the model)

### Requirement: Members are selected in priority order

The members step SHALL offer one selection at a time, phrased by position
("Select the FIRST member:", then SECOND, THIRD, …). The sequence of
picks SHALL define the member order. Each subsequent prompt SHALL list only
models not yet chosen, plus a completion option. At least one member SHALL be
required before the combo can be created; there is no upper bound.

#### Scenario: Pick sequence becomes the member order

- **WHEN** the user picks `anthropic:claude-sonnet-4.5` at the FIRST prompt
  and `glm:glm-4.7` at the SECOND prompt
- **THEN** the resulting combo's members SHALL be
  `["anthropic:claude-sonnet-4.5", "glm:glm-4.7"]` in that order

#### Scenario: A single-member combo is valid

- **WHEN** the user picks one model and chooses the completion option
- **THEN** the wizard SHALL proceed with exactly that one member

#### Scenario: Already-chosen models are not offered again

- **WHEN** the member prompt is shown after at least one pick
- **THEN** previously chosen models SHALL NOT be selectable
- **AND** a completion option SHALL be available once at least one member
  is chosen

#### Scenario: Pinned and unpinned forms of the same model are distinct members

- **WHEN** `glm:glm-4.7` (any account) has been chosen and the next member
  prompt is shown for a provider with multiple accounts
- **THEN** `glm@work:glm-4.7` (account `work`) SHALL still be selectable
- **AND** choosing it SHALL append a separate member, not deduplicate
- **AND** only a byte-identical member string SHALL be excluded from later
  prompts

#### Scenario: An empty member list cannot proceed

- **WHEN** no member has been picked yet
- **THEN** no completion option SHALL be offered and the wizard SHALL keep
  prompting until one member is chosen

### Requirement: Member options come from live topology state, account-aware

Member options SHALL be derived from the currently configured providers'
model whitelists, so every offered option is valid by construction. For a
provider declaring two or more accounts, each whitelisted model SHALL be
offered both unpinned (`provider:model` — any account; the provider's
account selection strategy applies) and pinned to each declared account
(`provider@account:model` — that connection's credential only). Providers
declaring zero or one account SHALL offer unpinned options only. The members
step SHALL explain the distinction in its prompt copy. When no providers or
whitelisted models exist, the command SHALL exit with an informational
message pointing at provider setup — it SHALL NOT render an empty picker.

#### Scenario: Options are whitelisted models

- **WHEN** the member step lists candidate models
- **THEN** every option SHALL correspond to a configured provider's
  whitelisted model in `provider:model` or `provider@account:model` form

#### Scenario: Multi-account provider offers pinned and unpinned options

- **WHEN** provider `glm` declares accounts `work` and `personal` and
  whitelists model `glm-4.7`
- **THEN** the member options SHALL include `glm:glm-4.7`, `glm@work:glm-4.7`,
  and `glm@personal:glm-4.7`
- **AND** picking `glm@work:glm-4.7` SHALL store the member verbatim with the
  `@work` pin

#### Scenario: Single-account provider offers unpinned options only

- **WHEN** a provider declares exactly one account (or none)
- **THEN** its models SHALL be offered in `provider:model` form only
- **AND** no `provider@account:model` options SHALL be offered for it

#### Scenario: No providers configured

- **WHEN** `combos add` runs and the topology declares no providers (or no
  whitelisted models)
- **THEN** the command SHALL exit with a message directing the user to add a
  provider first

### Requirement: Typed arguments remain a shortcut

All wizard inputs SHALL remain supplyable as typed arguments/flags
(`combos add <name> --members=... [--mode=...] [--capabilities=...]`), with
members in `provider:model` or `provider@account:model` form. When
every required value is supplied and interactivity is unnecessary, the command
SHALL create the combo without prompting. When a required value is missing and
no TTY is attached, the command SHALL fail with an error naming the missing
value and how to supply it.

#### Scenario: Fully-typed invocation skips prompts

- **WHEN** `tinyroute combos add coding --members=anthropic:claude-sonnet-4.5,glm:glm-4.7`
  is run
- **THEN** the combo SHALL be created without any interactive prompt

#### Scenario: Account-pinned members in typed arguments

- **WHEN** `tinyroute combos add coding --members=glm@work:glm-4.7,glm:glm-4.7`
  is run and `work` is a declared account of `glm`
- **THEN** the combo SHALL be created with the first member stored verbatim
  as `glm@work:glm-4.7`

#### Scenario: Missing value without a TTY errors clearly

- **WHEN** `combos add` is run without arguments and stdin is not a TTY
- **THEN** the command SHALL fail with an error naming the required values
  and an example invocation

### Requirement: Validation feedback inside the wizard

Name validation (allowed characters, no `:`, uniqueness against existing
combos) and member validation SHALL run during the wizard, re-prompting on
invalid input rather than aborting. The review step SHALL display the final
combo — including the numbered member order — before anything is written.

#### Scenario: Duplicate name re-prompts

- **WHEN** the user enters a combo name that already exists
- **THEN** the wizard SHALL show an error and re-prompt for the name

#### Scenario: Review shows numbered order before write

- **WHEN** the review step is reached
- **THEN** members SHALL be displayed numbered by position and the
  combo SHALL only be written after explicit confirmation

### Requirement: Combo members are validated against the topology at write

Configuration validation (`ValidateTopology`) SHALL reject a combo member
that is malformed (no `provider:model` structure), references an undeclared
provider, or pins (`@account`) an account the provider does not declare —
with `default` always permitted. Sub-combo members (a member naming another
combo) SHALL pass member validation. Every combo write path — the wizard, the
typed shortcut, and dashboard mutations — SHALL gate through this validation
before persisting.

#### Scenario: Member pinning an unknown account is rejected

- **WHEN** a combo is written with member `glm@ghost:glm-4.7` and `glm`
  declares no account named `ghost`
- **THEN** validation SHALL fail with an error naming the combo, the
  provider, and the account
- **AND** no configuration SHALL be persisted

#### Scenario: Member referencing an undeclared provider is rejected

- **WHEN** a combo is written with member `nosuchprov:some-model`
- **THEN** validation SHALL fail with an error naming the combo and the
  provider

#### Scenario: Sub-combo member passes validation

- **WHEN** a combo's member is the name of another declared combo
- **THEN** validation SHALL NOT reject that member on provider or account
  grounds
