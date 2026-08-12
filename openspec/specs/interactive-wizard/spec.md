# Interactive Wizard Specification

## Purpose

Provides a guided multi-step setup wizard for initial configuration, allowing users to interactively configure providers, credentials, and API keys through a sequential flow with navigation and validation.

## Requirements

### Requirement: Multi-step guided setup wizard
The system SHALL provide a guided setup wizard when `tinyroute init` is executed by default.

#### Scenario: Complete interactive setup flow (default behavior)
- **WHEN** user runs `tinyroute init` without any flags
- **THEN** system displays welcome message explaining the setup process
- **AND** system guides user through configuration steps sequentially
- **AND** system creates configuration files with user-provided values
- **AND** system validates configuration before completing

#### Scenario: Skip wizard with --no-interactive flag
- **WHEN** user runs `tinyroute init --no-interactive`
- **THEN** system creates default configuration without prompting (non-interactive mode)

### Requirement: Provider selection step
The system SHALL allow users to select and configure providers during setup wizard.

#### Scenario: Interactive provider selection
- **WHEN** setup wizard reaches provider configuration step
- **THEN** system displays available provider presets
- **AND** user can select one or more providers from the list
- **AND** system prompts for credential input after each provider selection

#### Scenario: Skip provider selection
- **WHEN** user chooses to skip provider selection
- **THEN** system creates configuration without providers
- **AND** user can add providers later via `tinyroute provider add`

### Requirement: Credential collection step
The system SHALL collect and validate provider credentials during setup wizard.

#### Scenario: Credential input after provider selection
- **WHEN** user selects a provider during wizard
- **THEN** system prompts for provider API credential with masked input
- **AND** system validates credential format (non-empty, correct length)
- **AND** system re-prompts if validation fails

#### Scenario: Optional credential entry
- **WHEN** user prefers to enter credentials later
- **THEN** system allows skipping credential entry
- **AND** wizard proceeds with provider configuration without stored credentials

### Requirement: API key creation step
The system SHALL allow users to create their first API key during setup wizard.

#### Scenario: Create default API key
- **WHEN** setup wizard reaches key creation step
- **THEN** system prompts for optional key name
- **AND** system generates new API key with provided or default name
- **AND** system displays generated key exactly once with storage instructions

#### Scenario: Skip key creation
- **WHEN** user chooses to skip key creation
- **THEN** system completes setup without creating default key
- **AND** user can create keys later via `tinyroute keys create`

### Requirement: Configuration validation step
The system SHALL validate complete configuration before finalizing setup wizard.

#### Scenario: Validate and display configuration summary
- **WHEN** user completes all setup wizard steps
- **THEN** system validates configuration (providers, routes, credentials)
- **AND** system displays summary of created configuration
- **AND** system shows generated API key (if created) with warning to store it

#### Scenario: Validation failure
- **WHEN** configuration validation fails during wizard
- **THEN** system displays specific validation errors
- **AND** system allows user to return to previous steps to fix issues
- **AND** wizard does not complete until configuration is valid

### Requirement: Wizard step navigation
The system SHALL allow users to navigate forward and backward through wizard steps.

#### Scenario: Navigate back to previous step
- **WHEN** user selects "Back" option during wizard
- **THEN** system returns to previous step with preserved values
- **AND** user can modify previous selections

#### Scenario: Navigate forward through wizard
- **WHEN** user completes current step and selects "Next"
- **THEN** system advances to next step in sequence
- **AND** system validates current step before proceeding

### Requirement: Wizard cancellation and cleanup
The system SHALL handle wizard cancellation without leaving partial configuration.

#### Scenario: User cancels wizard mid-flow
- **WHEN** user presses Ctrl+C during setup wizard
- **THEN** system prompts to confirm cancellation
- **AND** system removes any partially created files if cancellation confirmed
- **AND** system exits without side effects

#### Scenario: Wizard completion
- **WHEN** user completes all wizard steps and confirms
- **THEN** system finalizes configuration files
- **AND** system displays success message with next steps
- **AND** system shows generated API key (if applicable) with security warning
