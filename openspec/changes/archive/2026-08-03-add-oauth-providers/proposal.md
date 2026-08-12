## Why

tinyroute can only reach providers that accept a single static API key. The credential abstraction across the entire codebase is a `string` — `config.Provider.APIKey` → `proxy.ProviderInfo.APIKey` → `Dialect.AuthHeaders(cred string, ...)` — baked once at config load via `${VAR}` interpolation. There is no seam where a credential can be *resolved dynamically*. This blocks every provider whose auth is OAuth with short-lived, refreshable tokens: Codex (ChatGPT subscription), Claude (Claude Code OAuth), xAI/Grok, GitHub Copilot, Qwen, Kimi, GitLab, Kiro, Zed, Trae, Antigravity, Gemini CLI, and more. Users want to route to these using the account they already logged in with, not a separately-purchased API key.

Reference implementation: the colocated `9router/` codebase, which routes 40+ providers through exactly this OAuth-subscriber pattern.

## What Changes

- Introduce a `Credential` strategy interface (`Token(ctx) → (string, error)`) with `StaticKey` (today's behavior, zero change) and `OAuthRefreshable` implementations, resolved per-hop at `proxy.go:143` before `Dialect.AuthHeaders`.
- Add a credential **custodian** (`internal/credential`) that stores plaintext refresh tokens at mode `0600` with atomic `tmp+rename` writes — deliberately separate from `internal/auth`, which is a sha256 *oracle* that never needs plaintext back.
- Two-layer refresh dedup (per-credential singleflight + 10s result cache) with per-provider refresh profiles (body format, optional HTTP Basic header, optional `client_secret`), mirroring 9router's proven design.
- `tinyroute auth login <provider>`: interactive-first device-code + PKCE flows drawing OAuth metadata from presets, plus a manual-import escape hatch that works for any provider.
- `Dialect.AuthHeaders` gains OAuth awareness: an OAuth access token is emitted as `Authorization: Bearer`; the anthropic dialect falls back from `x-api-key` to `Bearer` when an OAuth token is present.
- A new **gemini** dialect (native Google protocol) as a third dialect alongside `anthropic` and `openai`, enabling Antigravity and Gemini CLI.
- New OAuth providers wired as presets — pilot: **xAI/Grok** via device-code — with a documented scope filter that declines non-LLM, gRPC, and web-cookie providers.
- A `tier` tag on presets (`free` / `freemium`), sourced from 9router's free/freeTier classification, surfaced in `provider add` and `provider list` so users can pick no-spend providers.

## Capabilities

### New Capabilities
- `provider-credentials`: dynamic credential resolution, refresh-token custodianship, refresh/concurrency, and the `auth login` / `auth import` flows.
- `gemini-dialect`: native Google Gemini request/response translation and auth.

### Modified Capabilities
- `provider-registry`: providers declare a `credential` block (static or oauth-refresh); presets carry per-provider OAuth metadata.
- `core-routing`: dialects emit the correct outbound auth shape for OAuth tokens; credential resolution occurs on the hop path before the upstream request is built.

## Impact

- `internal/credential/` (new): `Credential` interface, `StaticKey`, `OAuthRefreshable`, custodian store, refresh manager + dedup.
- `internal/config/topology.go`: `Provider` gains a `Credential` block; `APIKey`/`Headers` retained as the static shorthand (backward compatible).
- `internal/proxy/proxy.go`: `ProviderInfo` carries a `Credential` rather than a string; the hop resolves a token before `AuthHeaders`.
- `internal/core/interfaces.go`: `Dialect.AuthHeaders` semantics updated for OAuth-token awareness.
- `internal/dialect/anthropic/`, `internal/dialect/openai/`: `Bearer`-vs-`x-api-key` selection.
- `internal/dialect/gemini/` (new): native Google dialect.
- `internal/preset/`: preset schema extended with OAuth metadata; new OAuth presets added (pilot: xAI).
- `internal/cli/`: `tinyroute auth login` / `auth import` commands, interactive-first.

## Out of Scope

Deliberately declined to preserve tinyroute's identity as an LLM inference router:
- Non-LLM providers: web search (Brave/Serper/Tavily/Exa/...), TTS (ElevenLabs/Cartesia/Polly/...), image/video gen (Fal/Recraft/Runway/FLUX/...), embeddings (Jina/Voyage), scraping (Firecrawl).
- gRPC-transport providers: Cursor, Windsurf (would require a new transport layer).
- Web-cookie providers: Perplexity-web, Grok-web (browser-session model, wrong fit).
- The static-API-key preset batch (deepseek/groq/mistral/together/.../glm/baidu/tencent/...) — tracked separately as `expand-provider-catalog`, since it is a pure preset-data exercise with no new code.