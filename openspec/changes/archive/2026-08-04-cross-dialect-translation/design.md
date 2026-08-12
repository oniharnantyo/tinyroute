# Design: cross-dialect-translation

Port a proven, production-tested translation architecture into Go, adapted to tinyroute's
existing seams. The reference is 9router's `open-sse/translator/` (40+ providers, both
Anthropic and OpenAI inbound surfaces, full streaming translation). This design mirrors its
shape; implementation may translate its files near line-for-line.

## 1. Architecture: pivot through OpenAI as the intermediate format

```
                         ┌────────────────────────────────────────┐
   inbound surface       │  internal/translate/                   │
   (anthropic / openai)  │   registry.go    Register/Lookup       │
        │                │   state.go       StreamState           │
        ▼                │   concerns/      finishreason.go       │
   needsTranslation?     │                   usage.go             │
   ├─ no  → passthrough  │   request/  anthropic_to_openai.go     │
   └─ yes → translate    │   response/ openai_to_anthropic.go     │
        │   (direct route │            (chunk, *state) → []frame   │
        │    for the only  └────────────────────────────────────────┘
        │    pair we ship)            │
        ▼                             ▼
   proxy.POST(provider)   ← translated body
        │
        ▼
   relaySSE: upstream chunk → translateResponse(chunk, state) → frames → flush
             EOF: translateResponse(nil, state) → drain closing frames
```

**Why IR (not N×N per-pair translators):** with N dialects, IR needs 2N translators; per-pair
needs N(N−1). Concretely, once `openai↔anthropic` (this change) and `openai↔gemini`
(follow-on) exist, `anthropic↔gemini` composes for free via the two-hop pivot. OpenAI is the
natural IR because it is already tinyroute's dominant provider dialect.

**Direct route escape hatch:** the registry prefers an exact `from:to` translator (no
double-hop) when one is registered — for the pairs that are fragile through OpenAI (thinking
blocks, tool ids, non-base64 images). For this change the only pair is `anthropic↔openai`,
registered as a direct route.

## 2. The translator contract (Go)

```go
// internal/core/interfaces.go — replaces the existing request-only Translator stub.

// RequestTranslator converts a request body between dialects. Stateless.
type RequestTranslator interface {
    TranslateRequest(body []byte) ([]byte, error)
}

// ResponseTranslator converts one upstream chunk into zero or more outbound
// frames, mutating state. A nil chunk signals end-of-stream: emit any buffered
// closing frames. One method covers both mid-stream and drain — no Flush().
type ResponseTranslator interface {
    TranslateResponse(chunk []byte, state *StreamState) (frames [][]byte, err error)
}
```

```go
// internal/translate/registry.go

// Lookup resolves translators for a (from, to) pair. It returns a direct pair
// when registered, else composes from→openai→to. Returns ok=false when no path
// exists (the caller treats this as "cross-dialect routing unavailable").
func Lookup(from, to string) (req RequestTranslator, resp ResponseTranslator, ok bool)

func Register(from, to string, req RequestTranslator, resp ResponseTranslator)

func NeedsTranslation(from, to string) bool { return from != to }
```

`internal/route` stays import-pure: `Resolve` does not call `translate.Lookup` directly.
Instead `serve.go` injects a `Translatable func(from, to string) bool` predicate into the
`Router` (mirroring the existing `GetProvider`/`GetDialect` DI pattern), and the proxy calls
`translate.Lookup` for the actual translation. One predicate, consulted by both `Resolve` and
the proxy.

## 3. The streaming state machine (`StreamState`)

The stateful complexity of the whole feature lives in one struct, initialized once before the
stream and mutated by each chunk. Mirrors 9router's `initState`:

```go
// internal/translate/state.go
type StreamState struct {
    messageStartSent bool
    messageID        string        // synthesized "msg_…"; upstream id is "chatcmpl-…"
    model            string

    nextBlockIndex   int           // anthropic content blocks are ordered
    textBlockIndex   int
    textBlockOpen    bool
    thinkingBlockIndex int
    thinkingBlockOpen  bool

    toolCalls        map[int]toolCallState  // openai tool index → anthropic block
    toolArgBuffers   map[int][]byte         // buffered args, emitted sanitized at finish

    finishReason     string
    usage            *anthropicUsage        // captured from final openai usage chunk
}
```

This is *exactly* the block-bookkeeping the design investigation identified as the hard part —
and it is contained in one file, ~90 lines, with a 266-line reference
(`openai-to-claude.js`) to port from.

## 4. The three proxy seams (mapped to existing code)

The proxy today calls `hopDialect.RewriteModel` then sends, and relays the response
byte-for-byte (`relaySSE`/`relayDirect`). Translation inserts at three points:

| Seam | Today (proxy.go) | After |
|---|---|---|
| **Request** | `RewriteModel(body, hop.Model)` only | if `NeedsTranslation(inbound, hop)`: `TranslateRequest(body)` (sets target model); else `RewriteModel` (unchanged) |
| **Response, non-stream** | `relayDirect` (raw bytes) | if cross-dialect: read body, `TranslateResponse(body, state)`, write frames; else raw |
| **Response, stream** | `relaySSE` with a `UsageScanner` observer | generalize the observer into a chunk pipeline: per line → `TranslateResponse(chunk, state)` → encode frames → flush; EOF → `TranslateResponse(nil, state)` to drain |

`relaySSE` already does line-buffered reads and per-event flushing; the port *inserts a
translate step into the loop that already exists* plus the null-chunk drain at EOF. The
existing `UsageScanner` becomes the identity/passthrough case of the generalized pipeline.

## 5. `Resolve` and `Models`

- `Resolve` (router.go:110): replace `if prov.Dialect != surface { reject }` with
  `if prov.Dialect != surface && !translatable(surface, prov.Dialect) { reject }`. The
  existing mismatch error text is preserved for the genuinely-untranslatable case.
- `router.Models(surface)` is **unchanged**. It already filters through `Resolve`, so once
  `Resolve` permits translatable cross-dialect hops, `/anthropic/v1/models` lists every
  OpenAI-dialect whitelisted model automatically. No edit to the filter logic.

## 6. Fidelity policy and sharp edges

**Policy:** the OpenAI pivot is lossy by design for fields with no target equivalent; the
direct `anthropic↔openai` route avoids the double-hop but still drops genuinely-unmappable
fields. Unmapped fields are dropped with a debug log (never break the request). Direct routes
can be added per fragile pair later.

**Request mapping (`anthropic → openai`) — port from `claude-to-openai.js`:**

| Anthropic | OpenAI | Note |
|---|---|---|
| top-level `system` (string or block array) | first `{role:"system"}` message | join blocks; strip `x-anthropic-billing-header` artifact |
| mid-conversation `role:"system"` message | `{role:"user"}` wrapped in `<instructions>` | OpenAI rejects mid-chat system |
| `tools[].input_schema` | `tools[].function.parameters` | wrap in `{type:"function",function:{…}}` |
| `tool_choice:{type:"any"}` | `"required"` | OpenAI has no `"any"` |
| `content[].tool_use` | assistant `tool_calls[]` | `input` → `arguments` (JSON string) |
| `content[].tool_result` | `{role:"tool",tool_call_id,…}` | OpenAI requires a response for **every** tool_call — synthesize `"[No response received]"` stubs |
| `content[].image` (base64) | `image_url` data URI | |
| `max_tokens` | `max_tokens` | Anthropic requires it; OpenAI optional — pass through |

**Response/stream mapping (`openai → anthropic`) — port from `openai-to-claude.js`:**

| Concern | Decision |
|---|---|
| `message_start` usage | emit `{input_tokens:0, output_tokens:0}`; real usage (from final `usage` chunk) goes in `message_delta` at finish; fallback `{0,0}` |
| block ordering | `nextBlockIndex` counter; text/thinking/tool blocks open lazily, close on transition/finish |
| tool args | buffer across chunks, emit one sanitized `input_json_delta` at finish |
| `finish_reason` | centralized in `concerns/finishreason.go`: `stop→end_turn`, `length→max_tokens`, `tool_calls→tool_use`, … |
| reasoning | openai `reasoning_content` → anthropic `thinking` block (`thinking_delta`) |
| ids | synthesize `msg_…` / `toolu_…` (upstream ids are `chatcmpl-…`) |
| terminal event | Anthropic SSE terminates with `message_stop` (OpenAI-surface outputs synthesize `[DONE]` if omitted) |

**Robustness (port from `stream.js`):** skip non-JSON `data:` lines silently (upstreams
sometimes inject HTML/text errors mid-stream); terminal event emitted per dialect contract
(`message_stop` for Anthropic, `[DONE]` for OpenAI).

## 7. Testing strategy

- **Translator unit tests** (the load-bearing ones): recorded OpenAI SSE fixtures → assert
  the emitted Anthropic event sequence is valid (correct event order, block indices, ids,
  usage placement). Table-driven for request transforms. Property: round-trip
  `anthropic→openai→anthropic` is lossless for supported fields.
- **`router_test.go`:** update `Resolve` cross-dialect test — rejection now only when
  `translatable` is false; add a case where translatable is true and the hop resolves.
- **Proxy integration test:** anthropic-surface request to an openai-dialect stub provider,
  both streaming and non-streaming, asserts the client receives valid Anthropic output.
- Coverage for `internal/translate/` and the touched proxy/route code meets the 80% bar.

## 8. Out of scope

Gemini (follow-on change), openai-responses translation, server-side stateful features,
auth on `/v1/models`. See proposal Non-goals.
