# Interactive Wizard Specification (delta)

## MODIFIED Requirements

### Requirement: Multi-step guided setup in a TTY is a full-screen app

The system SHALL provide a guided setup app when `tinyroute init` is executed in
a TTY. In a TTY it SHALL run as a full-screen application supporting multi-provider
selection, credential entry per provider/account, back-and-forward navigation,
and validation, and SHALL write nothing to disk until the user confirms the final
summary. Without a TTY, `tinyroute init` SHALL fall back to the existing
non-interactive scaffold.

#### Scenario: Complete interactive setup flow (default behavior)

- **WHEN** user runs `tinyroute init` in a TTY without any flags
- **THEN** system displays a full-screen setup app explaining the setup process
- **AND** system guides user through configuration steps sequentially with back-forward navigation
- **AND** system validates configuration before completing
- **AND** system writes configuration files only after the user confirms

#### Scenario: Skip wizard with --no-interactive flag

- **WHEN** user runs `tinyroute init --no-interactive` or without a TTY
- **THEN** system creates default configuration without prompting (non-interactive scaffold)

### Requirement: Provider selection supports multiple providers

The system SHALL allow users to select and configure multiple providers during
setup, and to manage each provider's accounts.

#### Scenario: Interactive multi-provider selection

- **WHEN** setup reaches the provider configuration step
- **THEN** system displays available provider presets
- **AND** user can select one or more providers from the list
- **AND** system prompts for credential/account input after each provider selection

#### Scenario: Skip provider selection

- **WHEN** user chooses to skip provider selection
- **THEN** system creates configuration without providers
- **AND** user can add providers later via `tinyroute provider add`

### Requirement: Wizard cancellation and cleanup

The system SHALL handle setup-app cancellation without leaving partial
configuration, and SHALL write nothing to disk before the final confirmation.

#### Scenario: User cancels the setup app mid-flow

- **WHEN** user cancels (for example Ctrl+C) before confirming the final summary
- **THEN** no `config.json`, credential, or key file SHALL be written
- **AND** system exits without side effects

#### Scenario: Setup completion writes atomically on confirm

- **WHEN** user completes all steps and confirms the final summary
- **THEN** system finalizes configuration files atomically
- **AND** system displays success message with next steps
- **AND** system shows the generated API key (if applicable) with a security warning
