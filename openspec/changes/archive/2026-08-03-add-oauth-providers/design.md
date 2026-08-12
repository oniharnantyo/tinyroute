## Context

tinyroute's outbound credential is a single `string` threaded through three seams: `config.Provider.APIKey` (a `${VAR}`-interpolated value baked at load) → `proxy.ProviderInfo.APIKey` → `Dialect.AuthHeaders(cred string, headers)` at `proxy.go:143`. This works for static API keys but cannot express a credential that is *resolved dynamically* — which is exactly what OAuth demands: a short-lived access token obtained by refreshing a long-lived refresh token, on the request path.

The colocated `9router/` codebase solves this for 40+ providers. Its architecture maps almost 1:1 onto tinyroute's existing layers — the only layer tinyroute already has is the bottom one (`Dialect.AuthHeaders` = 9router's `BaseExecutor.buildHeaders`). Every layer above (OAuth flows → token store → refresh manager) is what this change adds.

Key 9router findings that shape the decisions below:
- Flows: authorization-code + PKCE (Claude, Codex, GitLab, …), device-code (xAI, Qwen, GitHub, Kimi, KiloCode, …), plus custom/proprietary (Kiro, Trae, Windsurf, Zed, Cursor).
- Refresh is not uniform: per-provider "profiles" differ in body format (JSON vs form-urlencoded), extra headers (iFlow uses HTTP Basic), and optional `client_secret`.
- Concurrency: a module-level lock keyed by `provider:tokenSuffix` **plus** a 10-second result cache.
- Refresh is proactive with per-provider lead times (Codex 5 days, Claude 4h, default 5min).
- Token store: SQLite at mode `0644`, unencrypted — a weakness we will **not** copy.

## Goals / Non-Goals

**Goals**
- Make the outbound credential a *strategy*, not a string, without changing behavior for existing static-key providers.
- Support OAuth-subscriber providers via device-code, PKCE, and manual import, with proactive refresh.
- Keep secrets at `0600`, atomic, never logged, never exposed after storage.
- Add a native gemini dialect so OAuth Google providers (Antigravity, Gemini CLI) are reachable.
- Stay interactive-first: `tinyroute auth login <provider>` runs with zero args in a TTY.

**Non-Goals**
- Becoming a universal gateway: no web-search / TTS / image / embedding / scraping providers.
- gRPC transports (Cursor, Windsurf) or web-cookie session providers (Perplexity-web, Grok-web).
- Inbound OAuth (validating external IdP tokens for *clients* of tinyroute) — separate future change.
- The static-API-key preset batch — separate change `expand-provider-catalog`.
- Auto-migration of existing `api_key` configs — they keep working unchanged.

## Decisions

**1. Credential is a strategy interface, resolved per hop.**
- **Decision:** Introduce `internal/credential` with `type Credential interface { Token(ctx) (string, error) }`. `ProviderInfo` carries a `Credential` instead of an `APIKey string`. At `proxy.go:143`, the hop calls `cred.Token(ctx)` and passes the resulting string to `Dialect.AuthHeaders`, whose signature stays `(cred string, headers)`. Implementations: `StaticKey` (returns the configured value) and `OAuthRefreshable` (returns a fresh access token, refreshing if stale).
- **Rationale:** The dialect owns *formatting* (`x-api-key` vs `Authorization: Bearer`, `anthropic-version`); the credential owns *obtaining* the secret. Keeping `AuthHeaders(cred string, …)` unchanged means the proxy resolves and the dialect formats — a clean split that touches the existing seam minimally.

**2. Custodian and oracle are separate packages.**
- **Decision:** `internal/credential` is a *custodian* — it holds plaintext refresh tokens and must hand them back to refresh. `internal/auth` stays a sha256 *oracle* (stores `sha256(plaintext)`, answers yes/no, never recovers plaintext). Do not overload `KeyStore`.
- **Rationale:** These are structurally different storage primitives. The oracle's irreversibility trick (digest storage) is inapplicable to refresh tokens, which must be usable. Conflating them would weaken both.

**3. Two-layer refresh dedup with per-provider profiles.**
- **Decision:** Layer 1 — a per-credential in-flight lock (singleflight keyed by `provider:tokenSuffix`) so concurrent requests share one refresh. Layer 2 — a 10-second successful-result cache so requests arriving shortly after a refresh reuse it. Refresh bodies are built from per-provider *profiles* (body format: `json` | `form`; optional HTTP Basic header; optional `client_secret`), mirroring 9router's `REFRESH_PROFILES`.
- **Rationale:** Singleflight collapses simultaneous refreshes; the 10s cache absorbs the burst that arrives seconds later. Profiles encode the real-world variation (Claude posts JSON with `client_id` only; iFlow adds Basic auth; GitHub conditionally includes `client_secret`) without a per-provider code path.

**4. Proactive refresh, with on-failure cooldown reused from the proxy.**
- **Decision:** Refresh *before* expiry using a configurable lead (default 5min; per-provider overrides, e.g. Codex 5d, Claude 4h). If a refresh fails or a refreshed token is rejected upstream with 401, classify it `FailureNoRetryWithCooldown` and reuse the existing `applyPenalty` 15-minute cooldown + loud warning — a chain cannot fix a bad refresh.
- **Rationale:** Proactive refresh avoids adding latency on the hot path for the common case. The failure taxonomy already accommodates auth errors, so no new penalty path is needed.

**5. `AuthHeaders` emits Bearer for OAuth tokens; anthropic falls back from x-api-key.**
- **Decision:** `Dialect.AuthHeaders` distinguishes a static key from an OAuth access token. For OAuth tokens, emit `Authorization: Bearer <token>`. The anthropic dialect keeps `x-api-key` for static keys but switches to `Bearer` when handed an OAuth token (matches 9router's `buildHeaders`). The proxy tags the resolved token with its kind so the dialect can choose.
- **Rationale:** Anthropic's API accepts a Claude OAuth access token as `Bearer`, not as `x-api-key`. Without this, `claude`/GitHub-Copilot-on-anthropic-surface/`kimi`-anthropic OAuth would all be broken at the last mile.

**6. Login UX: device-code + PKCE + manual import, interactive-first.**
- **Decision:** `tinyroute auth login <provider>` (zero args in a TTY → `Select` from OAuth-capable presets; single preset auto-selects; non-TTY → clear error). Device-code is the primary flow (no redirect server needed; works over SSH). PKCE with a localhost redirect is used where the provider requires it. `tinyroute auth import` is the always-works escape hatch: paste `access_token` + `refresh_token` + `client_id` + `token_endpoint` (or point at a native tool's credential file). Tokens are written to the custodian.
- **Rationale:** Device-code fits the CLI/loopback identity and the interactive-first rules (`cli-interactivity.md`). Import guarantees "any provider" works even when a flow is unsupported, revoked, or reverse-engineered drift occurs. Codex specifically needs a local proxy to intercept the `chatgpt.com` callback (9router spawns `:1455`); it is **not** the pilot — deferred to Phase 1.

**7. Config and preset extension, backward compatible.**
- **Decision:** `Provider` gains an optional `Credential` block. `api_key` / `Headers` remain as the static shorthand. A preset's OAuth metadata (`client_id`, authorize/token/device endpoints, scopes, flow type, refresh profile) is referenced by name, so a provider entry just says which preset it uses. Unknown credential types fail `ValidateTopology`.
- **Rationale:** Existing configs keep working byte-for-byte. Presets centralize the per-provider OAuth constants (which are the proprietary tools' public client ids) so provider entries stay terse and the constants live in one auditable place.

**8. Gemini as a third dialect.**
- **Decision:** Add `internal/dialect/gemini` implementing `core.Dialect` for Google's native protocol (translate request/response, `AuthHeaders` for both API-key and OAuth). This is what makes Antigravity and Gemini CLI reachable; the existing `gemini` preset (AI Studio OpenAI shim) continues to use the `openai` dialect.
- **Rationale:** Google-native OAuth providers do not speak OpenAI-compatible. A dedicated dialect is the honest representation. (Carvable into its own change if review prefers — it is self-contained.)

**9. Secrets at `0600`, atomic, never logged.**
- **Decision:** The custodian writes `credentials.json` with the existing `tmp+rename` atomic pattern at mode `0600` (reusing the technique from `WriteKeyFile`/`WriteTopology`). Refresh tokens, access tokens, and `client_secret`s are never written to logs; `provider list` shows only a masked "connected" indicator and expiry, never the token.
- **Rationale:** 9router stores these in SQLite at `0644` unencrypted — a direct violation of this repo's `security.md`. We do better than the reference.

**10. Scope filter: tinyroute stays an LLM router.**
- **Decision:** Decline non-LLM providers (search/TTS/image/embedding/scraping), gRPC providers (Cursor, Windsurf), and web-cookie providers (Perplexity-web, Grok-web). Document the filter in the preset catalog so the decision is auditable.
- **Rationale:** tinyroute's value is its narrowness — strict `provider:model` routing, two (now three) dialects, loopback-first. Every out-of-scope provider imported erodes that. The 9router catalog is most useful as a *filter*, not a checklist.

**11. Cost-tier tag, sourced from 9router's classification.**
- **Decision:** Presets gain a `tier` field — `"free"` (no credential, no cost) or `"freemium"` (free allocation, credential still needed) — mapped from 9router's `category: "free"|"freeTier"` + `hasFree` + `noAuth`. An optional `free_note` carries the limits text (e.g. gemini "15 RPM, 1M tokens/day on flash"). The tier renders as a tag in `provider add` and `provider list` next to the oauth/api-key tag.
- **Rationale:** tinyroute's category is already derived from credential type (oauth / api-key / none); cost is an orthogonal, small dimension best expressed as a single enum rather than mirroring 9router's three fields. The tag answers "can I try this without spending?" — the question that matters at selection time.

## Risks / Trade-offs

- **Risk: ToS and token revocation.** The OAuth client ids belong to the proprietary tools (Codex, Claude, …); using them from tinyroute may violate provider ToS, and providers (notably ChatGPT) actively detect and revoke non-official-client usage. *Mitigation:* none technical — document the trade-off; favor the manual-import path which reuses a token the user already obtained through the official tool.
- **Risk: Constant drift.** Endpoints, client ids, scopes, and refresh profiles change without notice. *Mitigation:* centralize them in presets (one auditable location); the import path degrades gracefully when a flow breaks.
- **Risk: Custodian becomes a high-value target.** A refresh token can mint access tokens indefinitely. *Mitigation:* `0600` atomic file; never logged; masked in all CLI output; loopback-default binding means it is not network-reachable.
- **Risk: Refresh on the hot path adds latency.** *Mitigation:* proactive refresh before expiry keeps the common case off the network; the 10s dedup cache bounds repeated refreshes during bursts.

## Appendix: Free / Freemium Providers (in-scope LLM, sourced from 9router)

Filtered to LLM inference that fits tinyroute's dialects — excludes 9router's TTS, search, image, embeddings, and gRPC providers. `tier: "freemium"` = free allocation but a credential is still required; `tier: "free"` = no credential, no cost.

**Freemium** (`tier: "freemium"`):

| Provider | tinyroute dialect | Auth | Free note (from 9router) |
|---|---|---|---|
| gemini | openai (AI Studio) | api key | 15 RPM, 1M tokens/day on flash |
| openrouter | openai | api key | 200 req/day, 27+ free models, no card |
| groq | openai | api key | fast inference, free tier |
| nvidia | openai | api key | free tier |
| cloudflare-ai | openai | api key + account id | free tier |
| vertex | gemini (native) | api key / oauth | Google free tier |
| kilo / kilo-gateway | openai | api key | free tier |
| kimchi | openai | oauth | free tier |
| bazaarlink | openai | api key | free tier |
| byteplus | openai | api key | ByteDance, free tier |
| poolside | openai | api key | free tier |
| ollama | openai | api key | 1 cloud model, resets every 5h/7d |
| api-airforce | openai | api key | free tier |
| nanobanana | openai | api key | free tier |

**Free** (`tier: "free"`):

| Provider | tinyroute dialect | Note |
|---|---|---|
| opencode | openai | tinyroute already has `opencode-zen` |
| mimo-free | openai | Xiaomi MiMo, no auth |
| ollama-local | openai | localhost, no auth |
| gemini-cli | gemini (native) | Google account (OAuth) |

Source mapping: 9router `category: "freeTier"` → `tier: "freemium"`; `category: "free"` / `noAuth: true` → `tier: "free"`. The `hasFree: true` flag is subsumed by these two values.