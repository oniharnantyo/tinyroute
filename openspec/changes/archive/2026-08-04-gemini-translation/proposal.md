## Why

`cross-dialect-translation` ships the translation registry and the `anthropic ↔ openai` pair.
Once a user configures a Gemini- or Vertex-dialect provider, the same clients should reach it
— and because the registry pivots through OpenAI, registering `openai ↔ gemini` makes
`anthropic ↔ gemini` compose for free (two-hop), with no direct translator written.

Gemini is structurally different enough from both OpenAI and Anthropic that it needs its own
pair translators and fidelity rules: function names must be sanitized to a Gemini charset and
restored on the response; tool calls/results use `functionCall`/`functionResponse` (an object,
not a JSON string); multi-turn thinking requires a `thoughtSignature`; and consecutive
same-role turns must be merged. tinyroute already has a partial, request-only
`gemini.Dialect.Translate` — this change supersedes it with the streaming-capable registry
form.

Depends on `cross-dialect-translation` (the registry, `StreamState`, proxy seams, and relaxed
`Resolve`).

## What Changes

- **Direct translators for `openai ↔ gemini`**, complete: request bodies, non-streaming
  responses, and stateful streaming. Ported from 9router's `request/openai-to-gemini.js` and
  `response/gemini-to-openai.js`.
- **Gemini-specific concerns**, factored under `internal/translate/concerns/`:
  `gemini_name.go` (sanitize + round-trip name map), and gemini handling in the shared
  `finishreason`/`usage`/`reasoning` concerns.
- **Migrate `gemini.Dialect.Translate`** into the new registry; remove the on-dialect method
  so all translation flows through one mechanism.
- **No new proxy or router work** — `Resolve`/`Models`/the three seams from the v1 change
  already handle any dialect pair the registry knows.

## Non-goals (recorded so they are not relitigated)

- **Vertex / Gemini-CLI / Antigravity envelope wrappers** (`project`, `requestId`,
  `request.sessionId`) are per-provider envelopes, not dialect translation. They belong in a
  provider/executor layer, not the translator. Out of scope here.
- **Direct `anthropic ↔ gemini` route** is not written; the two-hop pivot through OpenAI is
  accepted as sufficient (and lossy in documented ways). Add a direct route only if a pair
  proves fragile in practice.
- **`thoughtSignature` fidelity beyond a placeholder** is out of scope: a constant placeholder
  signature is injected, matching the reference. Real per-turn signature handling is a
  follow-on if multi-turn Gemini thinking degrades.

## Capabilities

### Added Capabilities

- `core-routing`: add the *Gemini dialect translation* requirement (the `openai ↔ gemini` pair
  and its fidelity specifics: name sanitization round-trip, `functionCall`/`functionResponse`,
  `thoughtSignature` placeholder, contents normalization).

## Impact

- **Code:** new `internal/translate/request/openai_to_gemini.go` and
  `internal/translate/response/gemini_to_openai.go`; new
  `internal/translate/concerns/gemini_name.go`; removal of
  `internal/dialect/gemini.Dialect.Translate` (and its test) in favor of registry
  registration.
- **Behavior change:** Gemini- and Vertex-dialect providers become reachable from the OpenAI
  surface directly, and from the Anthropic surface via the two-hop pivot. Existing same-dialect
  and anthropic↔openai behavior is unchanged.
- **Tests:** fixture-driven gemini translator tests; an integration test for
  anthropic-surface → gemini-provider via the two-hop pivot.
