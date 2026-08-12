## Why

When adding or configuring provider entries (e.g. via `tinyroute provider add`, `tinyroute auth set`, or setup wizards), `config.json` was previously written to disk before authentication flows (such as OAuth PKCE callback listening or interactive credential entry) completed. If a user interrupts the flow (e.g. with Ctrl+C) or an authentication step fails, tinyroute leaves unauthenticated or partial provider configuration persisted on disk.

## What Changes

- Defer `config.json` mutation during interactive provider setup (`tinyroute provider add`) until AFTER OAuth authentication finishes (if the user agrees to log in now).
- If an OAuth or interactive login flow is cancelled (Ctrl+C / SIGINT), times out, or fails with an error, do NOT apply or save the provider configuration to `config.json` or credential store.
- If the user explicitly declines immediate OAuth login when adding a provider, persist the unauthenticated provider configuration cleanly as requested.
- Ensure Ctrl+C (SIGINT) signal handling in OAuth PKCE and Device Code flows cancels context cleanly without leaving dangling server listeners or partial credentials.
- Ensure all interactive prompt commands (`auth set`, `auth import`, `agent install`, setup wizard) abort cleanly without persisting changes if interactive prompts are interrupted or return errors.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `provider-registry`: Ensure provider configuration persistence in `config.json` occurs atomically only after successful authentication or explicit user skip.
- `provider-credentials`: Ensure OAuth credentials and API key credentials are only saved to store when authentication succeeds.

## Impact

- `internal/cli/commands.go`: `cmdAdd` (`provider add`) deferred topology write logic and prompt error handling.
- `internal/cli/auth.go`: `runPKCEFlow` and `runDeviceCodeFlow` signal handling (`signal.NotifyContext`), ensuring Ctrl+C cancels uncompleted authentication and aborts config saving.
- `internal/cli/interactive/wizard.go`: Cancel setup cleanly on prompt error.
- Tests in `internal/cli/auth_test.go` and `internal/cli/commands.go` updated to verify aborted auth flows do not persist configuration.
