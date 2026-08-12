## Why

tinyroute exposes per-dialect inbound surfaces (`/openai/v1/*`, `/anthropic/v1/*`) but
routes only to providers whose dialect matches the surface. With no cross-dialect
translator, `Resolve` rejects every cross-dialect hop (router.go: "provider dialect does
not match surface"), so `/anthropic/v1/models` returns an empty list whenever no
`dialect: anthropic` provider is configured — the common case for users whose backends are
OpenAI-compatible (OpenRouter, Ollama, LM Studio, vLLM, opencode-zen, …). Anthropic-format
clients (Claude Code, Cursor in Anthropic mode) cannot use the gateway against those
backends, even though the two formats are mechanically interconvertible.

The active spec already names the escape hatch: *"This constraint holds until a
cross-dialect translator is provided."* This change provides one.

## What Changes

- **A translation registry** that pivots through **OpenAI as the canonical intermediate
  format**. Translators self-register `(from, to, requestFn, responseFn)`; `Lookup(from,
  to)` returns the pair or fails. A pair registered on an exact `from:to` runs as a
  **direct route**; otherwise the registry composes `from → openai → to` (two-hop).
  `needsTranslation(from, to) = from != to`.
- **Direct translators for the one critical pair — `anthropic ↔ openai` — complete**:
  request bodies, non-streaming responses, and stateful streaming SSE. Ported from a
  proven production reference (9router's `translator/request/claude-to-openai.js` and
  `translator/response/openai-to-claude.js`).
- **Three translation seams in the proxy:** (1) request body `inbound → provider` when
  dialects differ; (2) non-streaming response `provider → inbound`; (3) streaming response
  via a stateful chunk translator threaded through `relaySSE`, drained with a **null-chunk
  sentinel** at EOF (one function handles both mid-stream events and end-of-stream drain —
  no separate `Flush()`).
- **`Resolve` relaxes** its dialect guard: a cross-dialect hop is allowed when
  `translate.Lookup(surface, providerDialect)` succeeds, rejected with the existing
  mismatch error otherwise. `router.Models(surface)` is unchanged and now naturally
  surfaces every model whose provider is same-dialect **or** translatable.
- **Streaming fidelity decisions** (proven by the reference, adopted verbatim):
  `message_start` carries `usage:{input_tokens:0, output_tokens:0}`; the true usage
  (captured from OpenAI's final `usage` chunk, enabled by injecting
  `stream_options.include_usage`) is attached to `message_delta` at finish, falling back to
  `{0,0}` if absent. Tool arguments are buffered and emitted as a single sanitized
  `input_json_delta` at finish (not streamed incrementally). Outbound Anthropic SSE streams
  terminate with `message_stop` (OpenAI-surface outputs synthesize `data: [DONE]`).

## Non-goals (recorded so they are not relitigated)

- **Gemini translation** is a separate follow-on change (`gemini-translation`). Once
  `openai ↔ gemini` exists, `anthropic ↔ gemini` composes for free via the two-hop pivot —
  that is the point of the IR architecture.
- **The `openai-responses` dialect** (Responses API) is not translated here; it continues
  to pass through on its own surface.
- **Direct routes for fragile cross-dialect pairs beyond anthropic↔openai** are not added.
  The OpenAI pivot is accepted as lossy for unsupported fields (documented in the design)
  and a direct route can be added later per pair.
- **Server-side stateful features** (conversation memory, tool execution) remain out of
  scope — tinyroute translates and relays only.
- **Authentication of `/{surface}/v1/models`** is unchanged (loopback-exempt per the
  existing `models-endpoint-conformance` decision).
- **`gemini.Dialect.Translate`** (the only on-dialect translator today) is left in place;
  migrating it into the new registry is part of the gemini follow-on, not this change.

## Capabilities

### Modified Capabilities

- `core-routing`: relax *"Resolution is faithful to the inbound surface"* to permit
  translatable cross-dialect hops; add a cross-dialect scenario to model discovery.

### Added Capabilities

- `core-routing`: add the *Cross-dialect translation* requirement (registry, the three
  proxy seams, request/response/stream translation, and the fidelity policy).

## Impact

- **Code:** new `internal/translate/` package (registry, `StreamState`, `concerns/` for
  `finishreason` and `usage`); new `request/anthropic_to_openai.go` and
  `response/openai_to_anthropic.go` translators; new `core.RequestTranslator` /
  `core.ResponseTranslator` interfaces in `internal/core/interfaces.go`; surgical edits to
  `internal/proxy/progo.go` (the three seams; generalize `relaySSE`'s per-line observer
  into a chunk pipeline) and `internal/route/router.go` (relax `Resolve`).
- **Behavior change:** `/anthropic/v1/models` and `/anthropic/v1/messages` now serve
  OpenAI-dialect providers via translation. Same-dialect routing and the OpenAI surface are
  unchanged. `Resolve`'s error on a non-translatable cross-dialect hop is unchanged.
- **Tests:** fixture-driven translator tests (recorded OpenAI SSE streams → valid Anthropic
  event sequences), proxy integration tests for the cross-dialect streaming path, and
  updated `router_test.go` for the relaxed `Resolve`.
