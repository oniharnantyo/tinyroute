## ADDED Requirements

### Requirement: Provider configuration persistence is atomic and conditional on successful setup

When adding or updating a provider via interactive setup commands (`tinyroute provider add` / `tinyroute add`), configuration changes SHALL NOT be saved to `config.json` before interactive inputs and authentication flows complete, unless the user explicitly opts out of immediate authentication. If an interactive authentication flow is aborted, interrupted (SIGINT/Ctrl+C), or fails, `config.json` SHALL remain unchanged.

#### Scenario: Provider setup with immediate OAuth login saves config only on login success

- **WHEN** a user runs `tinyroute provider add <provider>` for an OAuth-capable provider
- **AND** confirms immediate OAuth login
- **AND** completes the OAuth login successfully
- **THEN** the provider entry SHALL be written to `config.json`
- **AND** the OAuth credential SHALL be saved to the credential store

#### Scenario: Aborted OAuth flow during provider add leaves config.json unchanged

- **WHEN** a user runs `tinyroute provider add <provider>` for an OAuth-capable provider
- **AND** confirms immediate OAuth login
- **AND** interrupts the OAuth flow (via Ctrl+C / SIGINT) or encounters a login failure
- **THEN** `config.json` SHALL NOT be mutated
- **AND** no credential record SHALL be saved

#### Scenario: Declining immediate OAuth login saves unauthenticated provider configuration

- **WHEN** a user runs `tinyroute provider add <provider>` for an OAuth-capable provider
- **AND** explicitly declines immediate OAuth login
- **THEN** the provider entry SHALL be written to `config.json` without credentials
- **AND** instructions to authenticate later SHALL be printed

#### Scenario: Interrupted interactive prompts leave config.json unchanged

- **WHEN** an interactive prompt during `provider add` or `auth set` is interrupted or returns an error
- **THEN** `config.json` SHALL NOT be saved or mutated
