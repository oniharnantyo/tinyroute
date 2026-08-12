## ADDED Requirements

### Requirement: OAuth flow cancellation discards credential updates

When an OAuth login flow (`runPKCEFlow` or `runDeviceCodeFlow`) is cancelled by context signal (such as SIGINT / Ctrl+C), timeout, or network failure, the command SHALL abort immediately, shut down any local HTTP listeners or polling loops, and SHALL NOT persist any token or credential record to the credential store.

#### Scenario: SIGINT during PKCE callback waiting aborts without saving

- **WHEN** an OAuth PKCE authorization is waiting for local HTTP callback
- **AND** a SIGINT (Ctrl+C) signal is received
- **THEN** the local HTTP listener SHALL shut down cleanly
- **AND** the command SHALL return `context.Canceled`
- **AND** no record SHALL be saved to the credential store

#### Scenario: SIGINT during device code polling aborts without saving

- **WHEN** an OAuth device code flow is polling for user authorization
- **AND** a SIGINT (Ctrl+C) signal is received
- **THEN** polling SHALL stop immediately
- **AND** the command SHALL return `context.Canceled`
- **AND** no record SHALL be saved to the credential store
