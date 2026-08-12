## Why

Existing LLM gateways for coding agents (9router, OmniRoute) solve a real problem — one stable
endpoint, automatic failover when a provider rate-limits — but they carry hundreds of bundled
provider definitions, web/PWA dashboards, MCP/A2A servers, token-compression layers, and
subscription-credential harvesting. Almost all of their mass is *data maintenance*, not routing
logic, and every upstream change needs a release.

tinyroute inverts that: ship the routing engine, and let the JSON config *be* the catalog. Think
`~/.ssh/config` for LLM endpoints. The target is a single static Go binary with zero dependencies,
around 1,350 lines for the first release, driven entirely from the terminal.

## What Changes

- **New Go service** (`tinyroute serve`) exposing an HTTP proxy on `127.0.0.1` that accepts
  `POST /v1/messages` (Anthropic Messages), `POST /v1/chat/completions` (OpenAI), and
  `GET /v1/models`.
- **Ordered fallback chains** per route. On connection failure, `429`, `5xx`, or `404`, the next
  provider in the chain is attempted. Failover is only possible before the first response byte
  reaches the client; this limit is explicit, not hidden.
- **Provider health state** with cooldowns persisted to `state.json`, so a rate-limited provider is
  skipped for a bounded window rather than retried on every request.
- **Split configuration.** `.env` holds deployment settings only (listen address, file paths,
  capture mode, cooldown defaults) and contains no secrets. `config.json` holds providers — including
  their inline `api_key` — and routes, and is therefore secret-bearing (`0600`, gitignored).
- **Provider presets as embedded data** for eight providers, consumed only by `tinyroute add`.
  Presets never participate at request time; `config.json` is the sole runtime source of truth.
  Adding a ninth provider costs zero lines of Go.
- **tinyroute-issued API keys** for inbound callers: minted by CLI, stored as `sha256` digests,
  with per-key model allow-lists, rate limits, expiry, and immediate revocation.
- **Session history** as an append-only JSONL index plus gzipped content-addressed blobs, capturing
  full request and response bodies to support later session lookup and replay.
- **Terminal-only interface.** No web UI, no TUI: `init`, `add`, `serve`, `validate`, `test`,
  `status`, `log`, `sessions`, `compact`, `keys`, `auth`.

Scoped to Anthropic-dialect providers in this change (`anthropic`, `zai`, `minimax`, `kimi`), which
Claude Code reaches by pure passthrough with no body translation. Reaching `openai`, `gemini`,
`zen`, and `kilo` from Claude Code requires an Anthropic→OpenAI translator, including a stateful SSE
transformer; that is a **required follow-on change**, deliberately separated so the config, key, and
history surfaces settle against real traffic before the riskiest code is written.

## Capabilities

### New Capabilities

- `service-config`: `.env` discovery and parsing, precedence against the real process environment,
  which settings are global versus per-provider, and the startup-only lifecycle.
- `provider-registry`: `config.json` schema for providers and routes, `${VAR}` interpolation,
  embedded presets, validation rules, atomic machine writes, and validate-before-swap hot reload.
- `request-routing`: inbound surfaces and their dialects, model-glob matching scoped by surface,
  chain resolution, failover classification, cooldown accounting, and client-native error mapping.
- `api-keys`: key format and minting, hashed storage, verification, per-key allow-lists and rate
  limits, expiry, revocation, and immediate effect on a running daemon.
- `session-history`: JSONL index record schema, content-addressed blob storage, session identity
  derivation, streaming usage capture, retention, and the `sessions`/`compact` commands.

### Modified Capabilities

None. This is the first change in the project.

## Impact

- **New code**: `cmd/tinyroute/` plus `internal/{core,config,dialect,route,proxy,history,auth,preset}`.
  Greenfield; the repository currently contains only `go.mod`.
- **Dependencies**: none added. `.env` parsing, SSE handling, and CLI dispatch are hand-rolled
  against the standard library. This is a hard constraint, not a preference.
- **New on-disk state** under `~/.tinyroute/`: `config.json`, `keys.json`, `state.json`,
  `requests.jsonl`, `blobs/`.
- **Security surface**: `config.json` holds plaintext provider keys (`0600`, gitignored). `blobs/`
  archives complete agent transcripts — source code, any `.env` files the agent read, secrets it
  encountered — in recoverable form (`0700`). The listener requires a bearer token by default.
- **Explicit non-goals**, recorded so they are not relitigated: subscription/OAuth credential
  reuse, per-provider Go adapters or quirk patches, bundled price tables and dollar budgets, web or
  desktop UI, MCP/A2A servers, token compression, Bedrock/Vertex, embeddings, BYOK passthrough,
  ACME/TLS automation, and OpenAI→Anthropic translation.
