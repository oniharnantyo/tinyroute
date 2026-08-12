## 1. Credential Framework (Phase 0 — the reusable core)

- [x] 1.1 Create `internal/credential/` package: `Credential` interface (`Token(ctx) (string, error)` + a token-kind tag), `StaticKey` impl (returns configured value, kind=static)
- [x] 1.2 Implement `OAuthRefreshable` credential: holds refresh_token + profile + cached access_token/expiry; `Token(ctx)` returns cached token or refreshes
- [x] 1.3 Implement the custodian store (`internal/credential/store.go`): load/save `credentials.json` at mode `0600` via atomic `tmp+rename`; never logs plaintext; hot-reloadable
- [x] 1.4 Implement refresh manager: per-credential singleflight lock (key `provider:tokenSuffix`) + 10s successful-result cache
- [x] 1.5 Implement per-provider refresh profiles (body format json|form; optional HTTP Basic header; optional `client_secret`) driven by preset metadata
- [x] 1.6 Unit tests: static returns immediately; refresh dedup collapses N concurrent calls to 1; 10s cache hit; refresh failure does not poison the cache; `credentials.json` written at `0600`

## 2. Wire the Seam (Phase 0 — prove it end-to-end)

- [x] 2.1 `config.Provider`: add optional `Credential` block; keep `APIKey`/`Headers` as static shorthand; `ValidateTopology` rejects unknown credential types
- [x] 2.2 `proxy.ProviderInfo`: carry a `Credential` instead of `APIKey string`; build it from `config.Provider` at wiring time
- [x] 2.3 `proxy.go:143`: resolve `cred.Token(ctx)` before `Dialect.AuthHeaders`; pass token-kind so the dialect can pick `Bearer` vs `x-api-key`
- [x] 2.4 `core.Dialect.AuthHeaders`: accept the token-kind; anthropic dialect emits `Authorization: Bearer` for OAuth tokens, `x-api-key` for static; openai dialect unchanged (`Bearer` either way)
- [x] 2.5 Confirm `applyPenalty` path: a refreshed-token 401 classifies as `FailureNoRetryWithCooldown` (15min) with no new penalty code
- [x] 2.6 Integration test: a provider with a static credential behaves exactly as before (regression); a provider with a mocked refreshable credential sends `Bearer` and refreshes on expiry

## 3. Pilot Provider — xAI/Grok (Phase 0)

- [x] 3.1 Add xAI OAuth preset metadata (client_id `b1a00492-073a-47ea-816f-4c329264a828`, device endpoint `https://auth.x.ai/oauth2/device`, token endpoint `https://auth.x.ai/oauth2/token`, OpenAI-compatible base URL)
- [x] 3.2 End-to-end manual test: `tinyroute auth login xai` → device code → token stored → a proxied request authenticates with the refreshed token

## 4. `auth login` / `auth import` CLI (Phase 1)

- [x] 4.1 `tinyroute auth login [provider]`: interactive-first (`Select` from OAuth-capable presets; single-candidate auto-select; non-TTY clear error); runs device-code flow, prints verification URI + user code, polls, stores token
- [x] 4.2 Implement PKCE flow (code_verifier/code_challenge S256) with localhost redirect for providers that require it
- [x] 4.3 `tinyroute auth import [provider]`: paste access+refresh+client_id+token_endpoint, or read a native tool's credential file; write to custodian
- [x] 4.4 `tinyroute auth status` / `provider list`: show masked "connected" indicator + token expiry; never print tokens
- [x] 4.5 Honor `--no-interactive`/`--force`; respect `cli-interactivity.md` checklist

## 5. Standard OAuth Provider Batch (Phase 1)

- [x] 5.1 Add OAuth presets + refresh profiles for the device-code/PKCE providers: qwen, github (Copilot), kimi (oauth variant), gitlab, grok-cli, kilocode, qoder, kimchi, iflow, codebuddy-cn, codebuddy-intl, trae, cline, clinepass, zed
- [x] 5.2 Add OAuth presets for the high-value PKCE providers: claude, codex
- [x] 5.3 Codex special-case: local proxy-callback (`:1455`-style) to intercept the `chatgpt.com` OAuth callback (or document manual-import as the supported path)
- [x] 5.4 Flag revoke-prone providers (claude, github) with a `RISK_NOTICE` note in the catalog/docs

## 6. Gemini Dialect (Phase 2)

- [x] 6.1 `internal/dialect/gemini/`: implement `core.Dialect` — `Paths`, `RewriteModel`, `AuthHeaders` (API key + OAuth Bearer), `InjectUsageOption`, `NewUsageScanner`, `WriteError`
- [x] 6.2 Request/response translation: OpenAI/Anthropic surface ↔ native Gemini generateContent
- [x] 6.3 Register the `gemini` dialect; add antigravity + gemini-cli presets
- [x] 6.4 Tests: round-trip translation; auth header selection; streaming relay

## 7. Documentation & Scope Filter

- [x] 7.1 Document the credential block in `docs/ARCHITECTURE.md` and a provider-auth reference
- [x] 7.2 Document the scope filter (declined categories: non-LLM, gRPC, web-cookie) in the preset catalog
- [x] 7.3 Document the ToS/revocation trade-off for OAuth-subscriber providers
- [x] 7.4 Update `.claude/rules/security.md` with the custodian/refresh-token storage requirements

## 8. Provider Cost-Tier Tag

- [x] 8.1 Add `tier` (`free` | `freemium`) and optional `free_note` to the preset schema; backfill the in-scope free/freemium presets from the 9router-sourced list (see `design.md` appendix)
- [x] 8.2 Render the tier as a tag in `provider add` (Select) and `provider list` / `auth status`, alongside the existing oauth/api-key tag
