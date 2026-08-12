## Context

When configuring LLM providers via `tinyroute provider add` (or `tinyroute add`), tinyroute prompts for configuration details (preset, custom instance name, base URL, API key or OAuth authentication).
Previously, `cmdAdd` wrote the new or modified provider topology to `config.json` via `config.WriteTopology` before asking whether to perform OAuth login or executing `cmdAuthLogin`.
When a user initiated an OAuth login flow (PKCE or device code) and then interrupted the process (e.g., via Ctrl+C / SIGINT while waiting for browser callback or device authorization), the CLI exited but left the newly added provider record in `config.json` without valid credentials.

## Goals / Non-Goals

**Goals:**
- Guarantee atomic configuration persistence: provider additions or modifications in `config.json` are only saved if the full interactive and authentication process succeeds, or if the user explicitly opts out of immediate login.
- Provide clean SIGINT (Ctrl+C) handling during OAuth flows so that uncompleted authentication attempts terminate cleanly without mutating `config.json` or credential stores.
- Apply this non-persisting transactional behavior across all interactive configuration entry points (`provider add`, `auth set`, `auth import`, `agent install`, setup wizard).

**Non-Goals:**
- Prevent users from adding unauthenticated providers when they explicitly decline immediate login ("Log in now? [N]").
- Change credential storage formats or OAuth token exchange protocols.

## Decisions

### 1. Defer Topology Persistence in `cmdAdd`
In `cmdAdd`:
- Collect preset parameters (name, base URL, dialect).
- If `p.OAuthCapable` and `isInteractive`:
  - Ask user if they wish to log in now.
  - If YES: execute `cmdAuthLogin` using a signal-aware context.
    - If `cmdAuthLogin` succeeds: add provider to topology map and call `config.WriteTopology`.
    - If `cmdAuthLogin` fails or is cancelled (Ctrl+C): do NOT update topology or call `config.WriteTopology`. Print a clear notice: "OAuth login cancelled or failed. Provider configuration was not saved." and return error.
  - If NO (user declines): add provider to topology map and call `config.WriteTopology`, informing user how to log in later.
- If `!p.OAuthCapable` (API key provider):
  - Check prompt returns for errors/interruptions (`interactive.Password`). If error occurs, do not write topology.

### 2. Signal Handling in OAuth Flows
In `runPKCEFlow` and `runDeviceCodeFlow`:
- Wrap input context with `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` to trap SIGINT/Ctrl+C.
- When signal fires, return `ctx.Err()` (e.g. `context.Canceled`), causing defer blocks to shut down local HTTP callback servers and cleaning up without calling `store.Save`.

### 3. Comprehensive Prompt Error Propagation
- Check prompt error returns across `auth set`, `auth import`, `agent install`, and `wizard.go`.
- Ensure any user interruption returns error and bypasses disk writes.

## Risks / Trade-offs

- **[Risk]** Users who cancel an OAuth flow will have to re-enter `tinyroute provider add <name>` from the beginning.
  - *Mitigation*: This is the expected transaction behavior—cancelling setup should discard unconfirmed changes.
