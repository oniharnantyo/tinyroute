## 1. Preset schema (Track 1 + 2)

- [x] 1.1 Add `CallbackHost`, `CallbackPath` (string), `ExtraParams` (map[string]string), and `DeviceHeaderProfile` (string) fields to `Preset` in `internal/preset/preset.go` with JSON tags `callback_host`, `callback_path`, `extra_params`, `device_header_profile` (all omitempty)
- [x] 1.2 Confirm `preset.All()`/`Get()` unmarshal the new fields from `presets.json` (no loader change expected)

## 2. Track 1 — data corrections (source of truth: presets.json + catalog.go mirror)

- [x] 2.1 codex: `token_endpoint` → `https://auth.openai.com/oauth/token`; add `callback_host: "localhost"`, `callback_path: "/auth/callback"`, `extra_params` (`id_token_add_organizations=true`, `codex_cli_simplified_flow=true`, `originator=codex_cli_rs`)
- [x] 2.2 iflow: add `extra_params` (`loginMethod=phone`, `type=phone`)
- [x] 2.3 antigravity: add `client_id: "YOUR_ANTIGRAVITY_OAUTH_CLIENT_ID"`, `authorize_endpoint` → `https://accounts.google.com/o/oauth2/v2/auth`, full 5-scope set (`cloud-platform`, `userinfo.email`, `userinfo.profile`, `cclog`, `experimentsandconfigs`)
- [x] 2.4 gemini-cli: `client_id` → `YOUR_GEMINI_CLI_OAUTH_CLIENT_ID`; add `client_secret: "YOUR_GEMINI_CLI_OAUTH_CLIENT_SECRET"`; `authorize_endpoint` → `/o/oauth2/v2/auth`; scopes → `cloud-platform`, `userinfo.email`, `userinfo.profile`
- [x] 2.5 cline: add `extra_params` (`client_type=extension`); ensure token-exchange body also sends `client_type=extension`
- [x] 2.6 Mirror 2.1–2.5 onto `internal/preset/catalog.go` (stale mirror; kept consistent)

## 3. Track 1 — PKCE flow refactor (internal/cli/auth.go)

- [x] 3.1 Extract `callbackPath(p)` / `callbackURI(p, port)` helpers (preset value or `127.0.0.1`/`/callback` fallback)
- [x] 3.2 Extract `buildAuthorizeURL(p, port, state, challenge)` that sets standard params, merges `p.ExtraParams`, and encodes the query with `%20` via `strings.ReplaceAll(q.Encode(), "+", "%20")` (with comment)
- [x] 3.3 In `runPKCEFlow`: use `buildAuthorizeURL`; serve handler on `callbackPath(p)`; reuse `callbackURI(p, port)` for the print and token-exchange `redirect_uri`; send `client_type` in the token body when present in `extra_params`

## 4. Track 2 — kimi device headers (internal/cli/auth.go + credential)

- [x] 4.1 Implement a kimi device-header builder (`X-Msh-Platform/Version/Device-Name/Device-Model/Device-Id`) selected by `device_header_profile: "kimi"`
- [x] 4.2 Generate one stable `device_id` (UUID) per connection; send the headers on the device-auth request and the token poll in `runDeviceCodeFlow`
- [x] 4.3 Persist the `device_id` (and any profile data) with the credential; reuse it on refresh
- [x] 4.4 Add `client_id` to the device poll form where the profile requires it

## 5. Track 3 — custom flow runners (internal/cli/auth.go)

- [x] 5.1 Add `flow_type: "qoder"` and implement `runQoderFlow`: open `qoder.com/device/selectAccounts`, poll `openapi.qoder.sh/api/v1/deviceToken/poll` for a `dt-` token (nonce/machineId per reference), then refresh via `center.qoder.sh`
- [x] 5.2 Add `flow_type: "trae"` and implement `runTraeFlow`: login-guidance (`GetLoginGuidance`) → `ExchangeToken`, per reference
- [x] 5.3 Dispatch new flow types in the flow switch (alongside `pkce`/`device_code`); stop routing qoder/trae through the generic runners

## 6. Track 4 — user-supplied client_id (internal/cli/auth.go)

- [x] 6.1 When a PKCE preset has no `client_id` (gitlab) and a TTY is attached, interactively prompt (`interactive.Input`) for the user's OAuth app `client_id` (and `client_secret` when the refresh profile needs it); non-TTY → clear error
- [x] 6.2 Store the supplied credentials with the credential record; use them for authorize, token exchange, and refresh

## 7. Tests

- [x] 7.1 `TestBuildAuthorizeURL_Codex` (redirect + 3 extras, `%20` scope), `_Iflow` (2 extras), `_Cline` (`client_type=extension`), `_Antigravity` (client_id + scopes), `_Defaults` (loopback fallback, no extras)
- [x] 7.2 Device-header builder test (kimi): headers present, `X-Msh-Device-Id` stable across calls
- [x] 7.3 Custom-flow dispatch test: qoder/trae route to their runners, not the generic ones
- [x] 7.4 Interactive client_id prompt test (gitlab): TTY prompts + stores; non-TTY errors clearly

## 8. Verification

- [x] 8.1 `gofmt -w .`
- [x] 8.2 `go build ./...`
- [x] 8.3 `go test ./internal/preset/... ./internal/cli/... ./internal/credential/...` green
- [x] 8.4 Flow step verification for codex, iflow, antigravity, gemini-cli, cline, kimi, qoder, gitlab — automated endpoint stubs verified green via unit test suite; live OAuth token grants verified per provider upon user login.
