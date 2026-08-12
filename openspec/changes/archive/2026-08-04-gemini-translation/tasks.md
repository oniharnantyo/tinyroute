Depends on `cross-dialect-translation` (registry, `StreamState`, proxy seams, relaxed
`Resolve`). TDD-ordered. Adds one pair to the registry and migrates the legacy
`gemini.Dialect.Translate` into it.

## 1. Fixtures and name round-trip (RED)

- [x] 1.1 `internal/translate/response/gemini_to_openai_test.go`: recorded Gemini fixtures
      (text part, `functionCall` part, `thought`+`thoughtSignature` part, `inlineData` image,
      `usageMetadata`, `finishReason`). Assert each yields valid OpenAI chunks; assert
      `STOP` with tool calls becomes `tool_calls`; assert names restore via `toolNameMap`.
- [x] 1.2 `internal/translate/request/openai_to_gemini_test.go`: assert generationConfig
      mapping (`top_p`→`topP`, `top_k`→`topK`, `max_tokens`→`maxOutputTokens`),
      `system`→`systemInstruction`, `assistant`→`model`, `functionCall`/`functionResponse`
      (with name recovery + non-object `{result}` wrap), name sanitization, and
      `normalizeGeminiContents` merging adjacent same-role turns.
- [x] 1.3 Name round-trip test: a tool name outside the Gemini charset survives a
      request→response cycle. Confirm RED.

## 2. Gemini name concern (GREEN)

- [x] 2.1 `internal/translate/concerns/gemini_name.go`: `Sanitize(name)` (enforce
      `[a-zA-Z_][a-zA-Z0-9_.:-]{0,63}`) and a per-request `NameMap` for restore. Port from
      `openai-to-gemini.js:sanitizeGeminiFunctionName` + the response `toolNameMap`.

## 3. Request translator: openai → gemini (GREEN)

- [x] 3.1 `internal/translate/request/openai_to_gemini.go`: port
      `request/openai-to-gemini.js` (`openaiToGeminiBase`). generationConfig, systemInstruction,
      message→contents (text/image/tool_call/tool_result), functionCall/functionResponse with
      name recovery, thoughtSignature placeholder on reasoning/tool turns, tools→
      `functionDeclarations`, `normalizeGeminiContents`.
- [x] 3.2 `Register("openai","gemini", req, nil)`. Confirm 1.2 passes.

## 4. Response + stream translator: gemini → openai (GREEN)

- [x] 4.1 `internal/translate/response/gemini_to_openai.go`: port
      `response/gemini-to-openai.js`. Per part: text vs `thought`→reasoning vs
      `functionCall`→tool_call vs `inlineData`→image; restore names via `toolNameMap`;
      `usageMetadata`→usage; `finishReason` mapping + STOP/tool_calls override. Satisfies the
      v1 `ResponseTranslator` contract (`chunk==nil` drains).
- [x] 4.2 `Register("gemini","openai", nil, resp)`. Confirm 1.1 passes.

## 5. Migrate the legacy on-dialect Translate

- [x] 5.1 Remove `Dialect.Translate` and its test from `internal/dialect/gemini/`. Verify
      nothing else references it (`grep`).
- [x] 5.2 Confirm the gemini dialect still satisfies `core.Dialect` (it must not implement
      the removed `Translator`/`Translate` surface); `go build ./...` clean.

## 6. Verify and accept

- [x] 6.1 `go test ./...` green; new translators meet the 80% coverage bar.
- [x] 6.2 `gofmt -w .` and `go vet ./...` clean.
- [x] 6.3 Two-hop integration test: anthropic-surface request → gemini-dialect stub provider,
      no direct `anthropic↔gemini` translator registered, asserts valid Anthropic output.
- [x] 6.4 Cross-check: each scenario in `specs/core-routing/spec.md` maps to a passing test
      (openai-surface reach, two-hop anthropic reach, name round-trip, tool structure,
      placeholder signature, contents merge).
- [x] 6.5 `openspec validate gemini-translation` clean.
