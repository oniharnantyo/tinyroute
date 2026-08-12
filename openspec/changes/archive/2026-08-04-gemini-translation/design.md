# Design: gemini-translation

Builds directly on `cross-dialect-translation`'s registry, `StreamState`, proxy seams, and
relaxed `Resolve`. No new architecture — this change adds one more pair to the registry.
Ported from 9router's `request/openai-to-gemini.js` and `response/gemini-to-openai.js`.

## 1. Why gemini is just another pair (and why that matters)

```
   anthropic ─┐                          ┌─ anthropic
              ▼                          │
            openai ◄──────────────────► gemini      ← this change adds openai↔gemini
              ▲                          │
   openai ────┘                          └─ (anthropic↔gemini composes FREE via two-hop)
```

Registering `openai ↔ gemini` immediately enables `anthropic ↔ gemini` through the registry's
two-hop pivot (`anthropic → openai → gemini` request; `gemini → openai → anthropic` response),
with no direct `anthropic ↔ gemini` translator written. This is the payoff of the IR
architecture established in the v1 change.

## 2. What makes gemini harder than anthropic (sharp edges)

Ported from the reference; each becomes a named concern or a branch in the translator.

| Sharp edge | Handling |
|---|---|
| **Function-name charset** `[a-zA-Z_][a-zA-Z0-9_.:-]{0,63}` | `concerns/gemini_name.go`: sanitize on request; keep a per-request `toolNameMap` to restore the original name on the response (stateful, not stateless). |
| **`thoughtSignature` for multi-turn thinking** | Inject a constant placeholder signature on assistant turns that carry reasoning or tool calls. Documented fidelity gap; real per-turn signature is a follow-on. |
| **Tool call structure** | OpenAI `tool_calls[].function.arguments` (JSON **string**) ↔ Gemini `parts[].functionCall.args` (**object**) via `tryParseJSON`. |
| **Tool result structure** | OpenAI `role:"tool"` messages ↔ Gemini `parts[].functionResponse.{id,name,response:{result}}`. Gemini requires the function **name** on the response (OpenAI has only `tool_call_id`) → build a `toolCallID→name` map upfront; wrap non-object results as `{result: x}`. |
| **`normalizeGeminiContents`** | Gemini rejects consecutive same-role turns → merge adjacent same-role `parts`. |
| **Param nesting + camelCase** | `temperature`/`top_p`/`top_k`/`max_tokens` → `generationConfig.{temperature, topP, topK, maxOutputTokens}`. (`top_k` survives openai→gemini, unlike openai→anthropic.) |
| **Role + container renames** | `assistant` → `model`; `system` → `systemInstruction.{role:"user", parts:[{text}]}`. |
| **finishReason override** | Gemini `STOP` with emitted tool calls → OpenAI `tool_calls` (so downstream anthropic translation emits `tool_use`). |

## 3. Migration of the existing `gemini.Dialect.Translate`

tinyroute's gemini dialect currently implements `Translate(body, from, to)` on the dialect —
request-oriented, not in the registry, and not streaming-capable. This change:

1. Ports the request-side logic into `internal/translate/request/openai_to_gemini.go` and
   registers it.
2. Adds the response/stream translator in
   `internal/translate/response/gemini_to_openai.go` (new — the old method had none).
3. Removes `gemini.Dialect.Translate` and its test; the dialect reverts to wire-format-only
   responsibilities (paths, headers, usage scanning), matching the anthropic/openai dialects.

After migration there is exactly one translation mechanism (the registry), consulted uniformly
by the proxy.

## 4. Fidelity

Same policy as v1: the OpenAI pivot is lossy for unmappable fields; unmappable fields are
dropped with a debug log, never fatal. The known gemini-specific losses are
`thoughtSignature` (placeholder) and any schema fields `cleanJSONSchema` strips. A direct
`anthropic ↔ gemini` route can be added later only if a real pair proves fragile — not
speculatively.

## 5. Testing

- **Translator unit tests:** recorded Gemini SSE/response fixtures (text, functionCall,
  thought + thoughtSignature, inlineData image) → valid OpenAI chunks; and OpenAI request
  fixtures → valid Gemini bodies (name sanitization, functionResponse with recovered name,
  contents normalization).
- **Name round-trip:** assert a tool name outside the Gemini charset survives a request→
  response cycle via the `toolNameMap`.
- **Two-hop integration:** anthropic-surface request → gemini-dialect stub provider, asserts
  valid Anthropic output without a direct `anthropic↔gemini` translator registered.
- Coverage for the new translators meets the 80% bar.
