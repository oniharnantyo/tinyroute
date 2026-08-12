TDD-ordered: write the contract and fixture tests first (RED), implement the registry and
translators (GREEN), then wire the proxy seams and relax `Resolve`. New `internal/translate/`
package; surgical edits to `core`, `proxy`, and `route`.

## 1. Contracts and fixtures (RED)

- [x] 1.1 In `internal/core/interfaces.go`, replace the request-only `Translator` stub with
      `RequestTranslator` (`TranslateRequest(body) ([]byte, error)`) and `ResponseTranslator`
      (`TranslateResponse(chunk []byte, state *translate.StreamState) ([][]byte, error)` —
      `chunk == nil` means drain). Add a `StreamState` type (or re-export from `translate`).
- [x] 1.2 Create `internal/translate/registry_test.go`: assert `Register` then `Lookup`
      returns the pair; assert `Lookup` composes `A→openai→B` when only `A→openai` and
      `openai→B` are registered; assert `NeedsTranslation(a,b) == (a != b)`; assert unknown
      pair → `ok=false`.
- [x] 1.3 Create `internal/translate/response/openai_to_anthropic_test.go` with recorded
      OpenAI SSE fixtures (plain-text stream, a tool-call stream, a stream with
      `reasoning_content`, a stream with final `usage`). Assert each emits a valid Anthropic
      event sequence: `message_start` first; `content_block_start`/`delta`/`stop` with
      correct `index`; synthesized `msg_…`/`toolu_…` ids; `message_delta` carrying usage at
      finish; `message_stop` last. Confirm RED (no impl yet).
- [x] 1.4 Create `internal/translate/request/anthropic_to_openai_test.go`: table-driven
      cases for system (string + block array), mid-conversation system→user, tools,
      `tool_choice:{type:"any"}→"required"`, `tool_use`→`tool_calls`, `tool_result`→`role:tool`,
      missing tool-response stub insertion, image base64→data URI. Confirm RED.

## 2. Registry and state (GREEN)

- [x] 2.1 `internal/translate/registry.go`: `Register(from,to,req,resp)`, `Lookup(from,to)`
      (direct pair, else compose via openai, else `ok=false`), `NeedsTranslation`. Exact
      mirror of 9router `translator/index.js:register/translateRequest/translateResponse`.
- [x] 2.2 `internal/translate/state.go`: `StreamState` with `messageStartSent`,
      `messageID`, `model`, `nextBlockIndex`, `textBlockIndex/textBlockOpen`,
      `thinkingBlockIndex/thinkingBlockOpen`, `toolCalls map[int]toolCallState`,
      `toolArgBuffers map[int][]byte`, `finishReason`, `usage`.
- [x] 2.3 `internal/translate/concerns/finishreason.go`: `FromOpenAIFinish(reason, target)`
      mapping `stop→end_turn`, `length→max_tokens`, `tool_calls→tool_use`, `content_filter`,
      `function_call`. Port from `concerns/finishReason.js`.
- [x] 2.4 `internal/translate/concerns/usage.go`: build Anthropic usage from OpenAI
      `usage`/`prompt_tokens_details` (input = prompt − cached − cache_creation).

## 3. Request translator: anthropic → openai (GREEN)

- [x] 3.1 `internal/translate/request/anthropic_to_openai.go`: port
      `request/claude-to-openai.js`. Implement `TranslateRequest`: system join + billing
      strip, message conversion (text/image/tool_use/tool_result), mid-chat system→user,
      `tools`/`tool_choice` mapping, missing-tool-response stub insertion. `Register` it.
- [x] 3.2 Confirm 1.4 passes.

## 4. Response + stream translator: openai → anthropic (GREEN)

- [x] 4.1 `internal/translate/response/openai_to_anthropic.go`: port
      `response/openai-to-claude.js`. Implement `TranslateResponse(chunk, state)`:
      emit `message_start` (placeholder `{0,0}` usage) on first chunk; open/close text and
      thinking blocks via `nextBlockIndex`; buffer tool args and emit one
      `input_json_delta` + `content_block_stop` at finish; capture usage from the final
      `usage` chunk; on `finish_reason` emit `message_delta`(usage, mapped stop_reason) +
      `message_stop`. On `chunk == nil`, drain any open block.
- [x] 4.2 Register the pair direct in both directions:
      `Register("anthropic","openai", req, nil)` and `Register("openai","anthropic", nil, resp)`.
- [x] 4.3 Confirm 1.3 passes (all four fixtures).

## 5. Wire the proxy seams

- [x] 5.1 In `internal/proxy/proxy.go`, resolve the inbound dialect (already on `reqCtx`)
      and the hop dialect; when `translate.NeedsTranslation(inbound, hopDialect)`, obtain
      translators via `translate.Lookup` and translate the request body in place of (or
      before) `RewriteModel`. Same-dialect path stays on `RewriteModel`.
- [x] 5.2 Generalize `relaySSE`'s observer: instead of only `UsageScanner.Observe`, thread a
      `ResponseTranslator` + `StreamState`; per upstream `data:` line → parse JSON →
      `TranslateResponse(chunk, state)` → encode each frame as `event:..\ndata:..\n\n` →
      write + flush. Skip non-JSON lines. At EOF call `TranslateResponse(nil, state)`.
- [x] 5.3 Non-streaming path: when cross-dialect, read the full body, `TranslateResponse`
      once, write the translated body; else `relayDirect` unchanged.
- [x] 5.4 Emit dialect-appropriate terminal SSE event at EOF (`message_stop` for Anthropic, `[DONE]` for OpenAI-surface outputs).

## 6. Relax Resolve and wire the predicate

- [x] 6.1 In `internal/route/router.go`, change `Resolve`'s dialect guard to allow the hop
      when `prov.Dialect == surface || translatable(surface, prov.Dialect)`. Take
      `translatable` from the `Router` (new field set in `New`).
- [x] 6.2 In `internal/cli/serve.go`, build the router with a `Translatable` predicate
      backed by `translate.Lookup`, and pass the same predicate to the proxy path. Single
      source of truth consulted by both `Resolve` and `Models`.
- [x] 6.3 Update `internal/route/router_test.go`: the cross-dialect rejection now holds only
      when `translatable` is false; add a case where it is true and the hop resolves.

## 7. Verify and accept

- [x] 7.1 `go test ./...` — all green; `internal/translate/`, `internal/proxy/`, and
      `internal/route/` meet the 80% coverage bar for touched code.
- [x] 7.2 `gofmt -w .` and `go vet ./...` clean.
- [x] 7.3 Proxy integration test: anthropic-surface request to an OpenAI-dialect stub
      provider, streaming and non-streaming, asserts valid Anthropic output.
- [x] 7.4 Manual smoke: `go run . serve`, then `curl -s localhost:8787/anthropic/v1/models`
      (lists the OpenAI-dialect provider models) and a streaming
      `POST /anthropic/v1/messages` against a configured OpenAI provider returns a valid
      Anthropic event stream.
- [x] 7.5 Cross-check: each scenario in `specs/core-routing/spec.md` maps to a passing test
      (translatable resolve, cross-dialect listing, request/response/stream translation,
      usage fidelity, unmappable-field drop).
- [x] 7.6 `openspec validate cross-dialect-translation` clean.
