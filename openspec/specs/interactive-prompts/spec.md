# Interactive Prompts Specification

## Purpose

Provides interactive prompt capabilities for CLI operations, including confirmation dialogs, masked input, and selection menus, with automatic non-interactive fallback for terminal environments and CI/CD pipelines.

## Requirements

### Requirement: Confirmation prompt before destructive operations
The system SHALL prompt users for confirmation before executing destructive operations by default (when running in a terminal).

#### Scenario: Confirm key revocation (default behavior)
- **WHEN** user runs `tinyroute keys revoke <key-id>` without any flags
- **THEN** system displays confirmation prompt with key ID
- **AND** user must explicitly confirm before key is revoked
- **AND** operation is canceled if user declines

#### Scenario: Skip confirmation with --no-interactive flag
- **WHEN** user runs `tinyroute keys revoke --no-interactive <key-id>`
- **THEN** system executes revocation immediately without prompt

#### Scenario: Skip confirmation with --force flag
- **WHEN** user runs `tinyroute keys revoke --force <key-id>`
- **THEN** system executes revocation immediately without prompt

### Requirement: Masked password input for sensitive credentials
The system SHALL mask user input when collecting sensitive credentials like API keys and tokens by default.

#### Scenario: Masked credential input (default behavior)
- **WHEN** user runs `tinyroute provider auth set <provider>` without any flags
- **THEN** system prompts for credential with masked input (asterisks)
- **AND** credential value is not visible on screen during entry

#### Scenario: Skip masked input with --no-interactive flag
- **WHEN** user runs `tinyroute provider auth set --no-interactive <provider>`
- **THEN** system reads credential from stdin without masking

### Requirement: Interactive selection from preset list
The system SHALL allow users to interactively select from available provider presets by default.

#### Scenario: Interactive preset selection (default behavior)
- **WHEN** user runs `tinyroute provider add` without preset name
- **THEN** system displays list of available provider presets
- **AND** user can navigate and select from the list
- **AND** selected preset is added to configuration

#### Scenario: Non-interactive preset selection
- **WHEN** user runs `tinyroute provider add --no-interactive <preset-name>`
- **THEN** system adds specified preset without displaying selection menu

### Requirement: Text input with validation
The system SHALL allow interactive text input with real-time validation for user-provided values.

#### Scenario: Interactive key name input
- **WHEN** user runs `tinyroute keys create --interactive`
- **THEN** system prompts for key name with input field
- **AND** system validates input is non-empty
- **AND** system re-prompts if validation fails

### Requirement: Terminal detection and graceful degradation
The system SHALL detect terminal capability and automatically degrade to non-interactive mode when appropriate, even when interactive is the default.

#### Scenario: Non-terminal environment
- **WHEN** command is run in non-terminal context (piped input, CI/CD)
- **THEN** system automatically skips all interactive prompts
- **AND** operation proceeds with default values or fails gracefully
- **AND** this automatic override takes precedence over default interactive behavior

#### Scenario: Terminal with --no-interactive flag
- **WHEN** command is run with `--no-interactive` flag in terminal
- **THEN** system skips all interactive prompts
- **AND** operation proceeds with default values

#### Scenario: Terminal with --force flag
- **WHEN** command is run with `--force` flag in terminal
- **THEN** system skips all interactive prompts (alias for --no-interactive)
- **AND** operation proceeds with default values

### Requirement: Interruptible prompts
The system SHALL allow users to interrupt interactive prompts with Ctrl+C.

#### Scenario: User cancels during confirmation prompt
- **WHEN** user presses Ctrl+C during confirmation prompt
- **THEN** system displays cancellation message
- **AND** operation is aborted without side effects

#### Scenario: User cancels during input prompt
- **WHEN** user presses Ctrl+C during text/password input
- **THEN** system displays cancellation message
- **AND** operation is aborted without partial data loss
