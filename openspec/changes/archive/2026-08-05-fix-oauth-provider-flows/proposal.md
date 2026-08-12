## Why

`tinyroute auth login` fails at the OAuth step for most providers. The provider
presets were ported from a reference router but the port was incomplete and has
decayed, leaving eight providers broken across four distinct root-cause tracks:

- **Track 1 — stale/missing preset data (PKCE + device):**
  - **codex** (PKCE) → `authorize_hydra_invalid_request`. Redirect URI
    (`http://127.0.0.1:1455/callback`) does not match the one registered for the
    borrowed client_id (`http://localhost:1455/auth/callback`); also missing three
    required params and using the wrong token endpoint.
  - **iflow** (PKCE) → missing `loginMethod=phone` / `type=phone`.
  - **antigravity** (PKCE) → `Missing required parameter: client_id`; also wrong
    authorize endpoint and 1 of 5 required scopes.
  - **gemini-cli** (device) → `invalid_client` / "client was not found". The
    preset's `client_id` is garbage
    (`694291880928-090c29f6s5q9m91o322…`) and must be
    `681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j…`; also missing
    `client_secret`, wrong endpoint, short scopes.
  - **cline** (PKCE) → `invalid or missing client_type parameter`. cline
    identifies by `client_type=extension`, not `client_id`.
- **Track 2 — missing device-flow headers:**
  - **kimi** (device) → `Missing user_code parameter`. Kimi's device endpoint
    requires `X-Msh-*` headers (notably a stable `X-Msh-Device-Id`) that tinyroute
    never sends.
- **Track 3 — non-standard flows:**
  - **qoder** (device) → HTTP 401. qoder is not RFC 8628: the user authorizes at
    `qoder.com/device/selectAccounts` and tinyroute must poll
    `openapi.qoder.sh` for a `dt-…` token. The generic device flow cannot reach it.
  - **trae** (PKCE-tagged) → uses a login-guidance + `ExchangeToken` flow, not an
    OAuth authorize redirect.
- **Track 4 — user-registered apps with no provider client_id:**
  - **gitlab** (PKCE) → `Missing required parameter: client_id`. GitLab Duo ships
    no client_id; the user must supply their own GitLab OAuth app credentials.

A single generic PKCE/device flow cannot satisfy all of these. The change makes
per-provider differences data-driven where possible (redirect shape, extra
params, client_type, device-header profile) and adds dedicated runners for the
genuinely non-standard flows (qoder, trae), plus an interactive path for
user-supplied client_ids (gitlab).

## What Changes

- **Generic preset fields (Track 1 + 2 + cline):** add `callback_host`,
  `callback_path`, `extra_params` (PKCE authorize), and a `device_header_profile`
  (device flows). Encode authorize-query spaces as `%20`.
- **Data corrections (Track 1):** codex (redirect + 3 params + token endpoint),
  iflow (2 params), antigravity (client_id + endpoint + scopes), gemini-cli
  (client_id + client_secret + endpoint + scopes), cline (`client_type=extension`
  + token-exchange body).
- **Kimi device headers (Track 2):** generate a stable per-connection
  `device_id`, send `X-Msh-*` headers on device-auth, token poll, and refresh;
  persist the `device_id` with the credential.
- **Custom flow runners (Track 3):** add `qoder` and `trae` flow types with
  dedicated implementations mirroring the reference; stop routing them through
  the generic PKCE/device runners.
- **User-supplied client_id (Track 4):** when a PKCE preset declares no
  `client_id` (gitlab), `auth login` SHALL interactively prompt for the user's
  OAuth app `client_id` (and `client_secret` where the refresh profile requires
  it), per the interactive-first CLI rule.
- Unaffected providers (claude, github, xai/grok-cli, qwen) keep current
  behavior; empty new fields fall back to today's defaults.

No **BREAKING** changes to persisted formats. Existing credentials remain valid.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `provider-credentials`: add requirements for (a) PKCE authorize-request shape
  (registered redirect URI, extra params, non-empty `client_id` for provider-owned
  clients, `%20` scope); (b) device-flow per-provider headers and a stable device
  id; (c) non-standard flow routing (qoder/trae run their own runners, not the
  generic ones); (d) interactive client_id acquisition for user-registered apps.
- `provider-registry`: add optional preset fields `callback_host`,
  `callback_path`, `extra_params`, and `device_header_profile`; correct the
  codex, iflow, antigravity, gemini-cli, and cline preset data; retype qoder/trae
  to their dedicated flow types.

## Impact

- `internal/preset/preset.go` + `presets.json` (+ `catalog.go` mirror) — new
  fields and corrected data.
- `internal/cli/auth.go` — PKCE authorize builder consumes new fields + `%20`;
  device flow gains header profiles + stable device id; new `runQoderFlow` /
  `runTraeFlow` runners; interactive client_id prompt for user-registered apps.
- `internal/credential/` — persist kimi `device_id` (and qoder nonce/machineId)
  alongside tokens for refresh reuse.
- Tests — authorize-URL construction (codex/iflow/cline/antigravity/gemini-cli),
  device-header builder (kimi), custom-flow happy paths (qoder/trae), interactive
  client_id prompt (gitlab).
- Out of scope / flagged: full qoder/trae refresh-token longevity; whether
  gemini-cli should be device vs PKCE (its reference uses authorize); broader
  client_secret hardcoding policy (security.md) — addressed per-secret as needed.
