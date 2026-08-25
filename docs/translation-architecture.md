# Translation Architecture

How tinyroute translates and proxies inference requests across LLM provider dialects.
This is the corrected, evidence-backed reference — it records the design as verified
against the code and specs, and explicitly pre-empts the plausible-but-wrong readings
that are easy to derive from fragments (see [Dead-ends to avoid](#dead-ends-to-avoid)).

## 1. Transport: hand-rolled HTTP, zero vendor SDKs

tinyroute talks to every upstream with the Go standard library — no provider SDKs
anywhere. The proxy builds each outbound request with `http.NewRequestWithContext`
and sends it with `deps.Transport.RoundTrip` (`internal/proxy/proxy.go:265,307`).
The proxy package imports `internal/core`, `internal/translate`, `internal/credential`,
and the standard library only.

This is not a Go-specific accident. Two independent TypeScript gateways studied as
references reach the same conclusion: **9router** (native `fetch` + `undici` for proxy
support only) and **OmniRoute** (`undici.fetch()` + `axios`, 290+ providers) both use
zero vendor SDKs. Three codebases, two languages, one answer.

**Why:** a gateway must own auth, transport, retries, and stream framing directly.
Vendor SDKs parse streams into typed objects and hide the transport — the wrong layer
for a proxy that needs transport-level failover and transparent SSE relay.

## 2. Strategy: OpenAI-canonical hub + escape pairs (NOT a neutral IR)

Cross-dialect translation pivots through OpenAI's format as the canonical intermediate
(`internal/translate/registry.go:12`, `const canonical = "openai"`).

`Lookup(from, to)` has a strict precedence:

1. **Direct pair** registered for `(from, to)` → lossless, one hop.
2. **Compose** `from → openai → to` via `composedReq` / `composedResp` → two hops.
3. Fail (`ok = false`) — routing treats this as "cross-dialect unavailable."

Translator count is therefore **O(N), not O(N²)**: each non-openai dialect needs ~4
spokes (request in/out, response in/out). Adding a dialect pair costs "1 package + 1
registry line" (`archive/2026-08-01-tinyroute-core-router/design.md:354`) — no proxy or
router changes. The translators are ported from 9router's proven implementations
(`internal/translate/request/anthropic_to_openai.go:2`).

**Escape pairs** are the safety valve: `Register(from, to, ...)` an exact pair and
`Lookup` prefers it over composition. Added on demand for fragile pairs where the
openai double-hop loses fidelity (see [§4](#4-costs-of-the-hub--when-escape-pairs-are-needed)).

> **No reference gateway builds a dedicated neutral IR.** All three (tinyroute, 9router,
> OmniRoute) pivot through OpenAI. Do *not* assume a "neutral IR graduation" lies ahead —
> the hub+escapes pattern scales to 290 providers, and nobody adopted the alternative.

## 3. The composition machinery — and the route-pair invariant

The registry is keyed by the **route pair** `(from = inbound client dialect,
to = outbound provider dialect)`, not by data-flow direction. This is the single most
misread invariant in the codebase:

| translator type | registered under `(from, to)` | converts (data flow) | vs. route |
|---|---|---|---|
| Request | `(from, to)` | `from → to` | **follows** the route (client → provider) |
| Response | `(from, to)` | `to → from` | **opposes** the route (provider → client) |

Responses oppose the route because they flow *backward* along it. Concretely:

- `openAIToAnthropic` converts **openai → anthropic**, but is registered under
  `("anthropic", "openai")` (`response/openai_to_anthropic.go:19`). Route pair =
  anthropic-client → openai-provider; response data flow = to→from = openai→anthropic. ✓
- `geminiToOpenAI` converts **gemini → openai**, registered under `("openai", "gemini")`
  (`response/gemini_to_openai.go:13`). Route = openai-client → gemini-provider;
  response data flow = gemini→openai. ✓

This is why `composeReq` and `composeResp` look asymmetric (their `Lookup` arguments
differ) while both are correct: response composition reverses the spoke lookup precisely
*because* response translators oppose the route. The golden test
(`internal/translate/registry_test.go:46`, asserting `"chunk-OtoB-AtoO"` hop order) is
the proof — it would fail if the spokes were swapped.

## 4. Costs of the hub — and when escape pairs are needed

Composition is universal but not free. The costs, in increasing severity:

1. **Two hops of runtime work** on every composed path.
2. **Expressiveness ceiling.** Any feature OpenAI's chat-completions format cannot
   represent is dropped on hop 1, before the target dialect sees it:
   - *Thinking budget* is quantized to OpenAI's 5-level `reasoning_effort` enum, then
     re-expanded — a continuous token count cannot round-trip.
   - *Prior-turn thinking blocks* (reasoning replay) have no field in OpenAI's *request*
     schema → dropped.
   - *Tool-call identity* is flattened (args become a JSON string; results become
     separate `role:"tool"` messages matched only by id) and must be reconstructed.
3. **Streaming state coherence** across two hops (block lifecycle, tool-arg accumulation)
   is fragile; see [§5](#5-response-streaming--state-threaded-long-tail-bearing).

**Escape pairs bypass the hub** for specific fragile routes. Reference example:
OmniRoute's `openai-sse/translator/request/claude-to-gemini.ts` (264 LOC) is a direct
Claude→Gemini translator that preserves thinking budget, prior-turn thinking blocks,
tool-call identity, and system structure — everything the openai double-hop would lose.
It is registered *only* for the plain Gemini API (envelope-wrapped variants fall back to
the hub). tinyroute currently has zero escape pairs; the hub is accepted as lossy and
escapes are added per-pair on demand (`archive/2026-08-04-cross-dialect-translation/proposal.md`,
Non-goals: *"Direct routes for fragile cross-dialect pairs beyond anthropic↔openai are not
added … a direct route can be added later per pair"*).

**Trigger for adding an escape pair:** a specific composed route drops a feature a user
notices. Not before.

## 5. Response streaming — state-threaded, long-tail-bearing

Streaming translation is a state machine, not a mapper. `core.StreamState`
(`internal/core/streamstate.go:35`) is the mutable accumulator threaded through every
chunk (and the nil/drain call at end-of-stream):

- **Block lifecycle:** `TextBlockOpen` / `ThinkingBlockOpen` / `NextBlockIndex` — Anthropic
  requires explicit `content_block_start`/`_stop` events that span many deltas.
- **Tool-call accumulation:** `ToolCalls map[int]ToolCallState` + `ToolArgBuffers` —
  args arrive as fragments, buffered per index, emitted as a single sanitized
  `input_json_delta` at finish (`response/openai_to_anthropic.go:135-143`).
- **Cache tokens:** extracted from OpenAI's nested `prompt_tokens_details`
  (`concerns/usage.go`) → Anthropic's first-class `cache_read/creation_input_tokens`.
- **Thinking/reasoning:** `reasoning_content` → `thinking_delta`.
- **Finish reason / usage / EOS drain:** mapped and flushed at end-of-stream.

tinyroute's `openAIToAnthropic` translator handles the **core long tail at parity** with
OmniRoute (ported from 9router). The EOS drain is cross-validated: tinyroute's
`composedResp` nil-chunk drain (`registry.go:124-130`) is the same pattern as OmniRoute's
null-flush (`open-sse/translator/index.ts:627-635`) — both call the second hop with `null`
so it can emit terminal events (final usage, stop signals) even when the first hop produced
no intermediate output. Two independent implementations converged on identical edge-case
handling.

**Tool-arg streaming nuance:** args are buffered and emitted as one `input_json_delta` at
EOS, not incrementally. This is spec-compliant (clients concatenate `partial_json` and
parse at `content_block_stop`) and gains robustness via `sanitizeToolArgs`; the only
observable difference is progressive rendering of tool args. Deliberate trade, not a bug.

The *extreme* tail (models emitting tool calls as XML, provider-specific shims,
internal-reasoning placeholders) is not handled — it's a per-weird-provider concern,
addressed reactively exactly as the references did.

## 6. Reachability — and why the gaps are by design

The translation surface today, gated by `translate.CanTranslate` wired into routing
(`internal/cli/serve.go:154`):

```
                       upstream provider dialect
                       openai    anthropic    gemini    openai-responses
 client surface  ┌──────────────────────────────────────────────────────┐
 openai       ───┤   ➖         ❌          ✅          ❌
 anthropic    ───┤   ✅         ➖          ✅          ❌
 gemini       ───┤   ❌         ❌          ➖          ❌
 openai-resp  ───┤   ❌         ❌          ❌          ➖
                 └─  ➖ = passthrough   ✅ = translatable   ❌ = blocked

   ✅ paths: A→O (direct) · O→G (direct) · A→G (composed via openai)
```

**Unsupported combos fail loudly, never silently.** The router's `FaithfulSurfaceGuard`
(`internal/route/router_test.go:182`, logic at `router.go:202-203`) returns a named error
— `"provider %q dialect %q does not match surface %q"` — rather than attempting a
non-existent translation. A surface serves only what it can translate *faithfully*.

**The asymmetry is a complete product stance, not a work-in-progress.** The primary use
case is an **Anthropic-surface client (Claude Code, Cursor in Anthropic mode) reaching
OpenAI-compatible / Gemini backends** (`archive/2026-08-04-cross-dialect-translation/proposal.md`,
Why). That single sentence explains the entire A→O→G chain.

The reverse directions are **explicit non-goals**, not queued work:

- **O→A (OpenAI→Anthropic)** — *not* a future milestone. Listed in the core-router
  Out-of-scope section: *"OpenAI→Anthropic translation ('M2b'). Clients that speak OpenAI
  are already served by OpenAI-dialect providers."* An OpenAI-speaking client has a
  surplus of OpenAI-dialect backends; there is no scarcity problem routing them to an
  Anthropic upstream would solve. No ratified spec requires it.
- **openai-responses translation** — pass-through *by design*
  (`openspec/specs/core-routing/spec.md:112-114`): *"The endpoint SHALL behave as a thin
  pass-through: server-side stateful features (`store`, `previous_response_id`,
  `conversation`, `background`, and built-in tools) are not provided by tinyroute."* The
  Responses surface exists for **Codex** (`wire_api = "responses"` → `/openai/v1/responses`),
  which speaks the Responses API natively and reaches Responses-speaking backends via
  route + model-rewrite + SSE relay + usage extraction — without body translation.

## 7. Extending the translation layer

| Goal | Mechanism | Cost |
|---|---|---|
| Add a dialect pair to the hub | new translator package + `Register(from, to, ...)` in `init()` | 1 package + 1 registry line; no proxy/router changes |
| Add a lossless escape pair | `Register(from, to, ...)` for an exact pair | `Lookup` prefers it automatically |
| Add a new client surface | implement `core.Dialect` + `dialect.Register` | mounts its own inbound path |

The seams are deliberately stable: the proxy and router never change when a dialect or
translator is added. The SSE translator is the project's designated risk boundary
(`archive/2026-08-01-tinyroute-core-router/design.md:366`: *"The SSE translator is the
project's whole risk"*).

## Dead-ends to avoid

Plausible readings that are **wrong**, recorded so they are not re-derived:

- ❌ *"tinyroute does direct pairs → will hit O(N²)."* → No. Canonical-hub composition is
  built into `translate.Lookup` (`registry.go:46-65`); translator count is O(N).
- ❌ *"The hub is lossy → graduate to a neutral IR."* → No reference gateway does this.
  Hub + escape pairs scales to 290 providers (OmniRoute). The trigger for an escape pair
  is a *specific* route losing a feature, not dialect count.
- ❌ *"`composeResp`'s spokes look swapped — a bug."* → No. The route-pair invariant
  (responses oppose the route) makes the spoke order correct; `registry_test.go:46` is the
  golden master that proves it.
- ❌ *"O→A is queued milestone 'M2b'."* → No. M2b is an **explicit non-goal** with a
  stated rationale (OpenAI clients are already served by OpenAI-dialect providers).
- ❌ *"openai-responses translation is deferred."* → No. Pass-through **by design** (spec
  mandate); the Responses API's stateful surface is deliberately not translated.

## Key references (file:line)

- Transport (hand-rolled HTTP): `internal/proxy/proxy.go:265,307`
- Canonical hub + composition: `internal/translate/registry.go:12,46-89`
- Route-pair invariant (registrations): `response/openai_to_anthropic.go:19`,
  `response/gemini_to_openai.go:13`
- Composition golden test: `internal/translate/registry_test.go:46`
- Streaming state: `internal/core/streamstate.go:35`
- FaithfulSurfaceGuard (loud rejection): `internal/route/router.go:202-203`,
  `internal/route/router_test.go:182`
- Routing gate wiring: `internal/cli/serve.go:154`
- O→A non-goal: `openspec/changes/archive/2026-08-01-tinyroute-core-router/design.md` (Out-of-scope)
- openai-responses pass-through: `openspec/specs/core-routing/spec.md:112-114`
- Cross-dialect change (Why + Non-goals): `openspec/changes/archive/2026-08-04-cross-dialect-translation/proposal.md`
- Escape-pair reference: OmniRoute `open-sse/translator/request/claude-to-gemini.ts`