TDD-ordered: write the invariant test first (RED), make it pass (GREEN), then verify.
Standard library only; surgical edits to `core`, `route`, `dialect/*`, and `cli`.
The proxy (`internal/proxy`) is intentionally untouched — its attempt loop is
dialect-agnostic and stays so.

## 1. Split the dialect interface (RED → GREEN)

- [x] 1.1 Add `MountPaths() []string` and `WriteModels(w http.ResponseWriter, ids []string)`
      to `core.Dialect` in `internal/core/interfaces.go`. Correct the stale doc comment
      on `Paths()` (it is the **outbound/upstream** path, not inbound).
- [x] 1.2 Implement `MountPaths()` on `openai` (`["/openai/v1/chat/completions"]`) and
      `anthropic` (`["/anthropic/v1/messages"]`). Leave their `Paths()` (outbound) unchanged.
- [x] 1.3 Implement `WriteModels` on `openai` by moving the existing
      `WriteModelsResponse` logic behind the interface method (constant `created: 0`,
      `owned_by: "tinyroute"`).
- [x] 1.4 Remove the dead `dialect.ByPath` (exported, zero callers) and switch the
      registry's path indexing to `MountPaths()`.
- [x] 1.5 Unit-test: each dialect reports the expected `MountPaths()` and outbound
      `Paths()` independently.

## 2. Mount namespaced surfaces (GREEN)

- [x] 2.1 In `internal/cli/serve.go`, mount `d.MountPaths()` instead of `d.Paths()` in
      the registry-driven loop. Confirm outbound callers (`proxy.go`, `commands.go`) are
      untouched.
- [x] 2.2 Replace the hand-mounted `GET /v1/models` handler with a per-dialect
      `GET {surface}/v1/models` loop calling `d.WriteModels(w, router.Models(d.Name()))`.
- [x] 2.3 Add a faithful-surface `/models` test per surface (OpenAI + Anthropic): every
      returned `id` resolves on that surface; the un-namespaced `/v1/models` returns 404;
      non-GET returns 405.

## 3. Faithful-surface resolution (RED → GREEN)

- [x] 3.1 In `internal/route/router.go` `Resolve`, after resolving a hop, reject it if
      the provider's dialect ≠ the inbound `surface`. Wrap the error with `fmt.Errorf`.
- [x] 3.2 Test (RED first): on the OpenAI surface, `openai:gpt-4o` resolves;
      `anthropic:claude-3-5-sonnet` is rejected with a clear error and makes no upstream
      call. Confirm `Models("openai")` still returns only OpenAI-resolvable ids.

## 4. Anthropic `/models` writer (P1 — honest-minimal)

- [x] 4.1 Implement `WriteModels` on `anthropic`: native shape (`type: "model"`,
      `display_name` = `id`, `created_at` = epoch `1970-01-01T00:00:00Z`,
      `max_input_tokens`/`max_tokens` omitted, no capabilities, `has_more: false`).
      Accept and ignore `after_id`/`before_id`/`limit` query params.
- [x] 4.2 Test: shape is native; `created_at` is the constant epoch across requests;
      every `id` resolves on the Anthropic surface.

## 5. OpenAI Responses dialect (GREEN)

- [x] 5.1 New package `internal/dialect/openairesponses`: `Name() → "openai-responses"`,
      `Paths() → ["/v1/responses"]` (outbound), `MountPaths() → ["/openai/v1/responses"]`,
      `ParseRequest` (reads `input`/`instructions`/`model`/`stream`),
      `RewriteModel`, `AuthHeaders` (Bearer, same as openai), `WriteError`,
      `InjectUsageOption` (no-op), `WriteModels` (Responses-native or openai-shaped —
      decide; lean openai-shaped for the listing since it is the OpenAI vendor surface).
- [x] 5.2 Usage scanner: read `response.usage` from the `response.completed` SSE event
      ("last chunk carrying usage wins"). Capture `output_tokens_details.reasoning_tokens`.
- [x] 5.3 Add `ReasoningTokens int64` to `core.Usage`; leave the OpenAI chat scanner's
      value at `0`.
- [x] 5.4 Register the dialect; add the import in `serve.go`. Confirm `serve.go`'s mount
      loop serves `/openai/v1/responses` with no further wiring.
- [x] 5.5 Tests: routes by `model`; non-streaming relays in Responses shape; streaming
      relays SSE verbatim and extracts usage from `response.completed`; reasoning tokens
      captured when upstream reports them.

## 6. Remove legacy paths and update tests

- [x] 6.1 Confirm `/v1/chat/completions`, `/v1/messages`, `/v1/models` return 404.
- [x] 6.2 Update request-URL strings in the six `*_test.go` files
      (`core/types_test.go`, `proxy/proxy_test.go`, `config/catalog_test.go`,
      `history/sqlite/store_test.go`, `accesslog/accesslog_test.go`,
      `dialect/openai/models_test.go`) to the namespaced surfaces. Do **not** touch
      outbound paths in `catalog.go` or dialect `Paths()`.

## 7. Verify and accept

- [x] 7.1 `go test ./...` — all green; touched packages meet the 80% coverage bar.
- [x] 7.2 `gofmt -w .` and `go vet ./...` clean.
- [x] 7.3 Manual smoke: `go run . serve`, then
      `curl -s localhost:8787/openai/v1/chat/completions …`,
      `curl -s localhost:8787/anthropic/v1/messages …`,
      `curl -s localhost:8787/openai/v1/models | jq`,
      `curl -s localhost:8787/anthropic/v1/models | jq`,
      `curl -s localhost:8787/openai/v1/responses …`, and confirm
      `curl -s localhost:8787/v1/chat/completions` → 404.
- [x] 7.4 Cross-check: each scenario in `specs/core-routing/spec.md` maps to a passing
      test (per-surface listing + native shape, legacy 404, 405, faithful resolution,
      Responses route/relay/usage).
- [x] 7.5 `openspec validate --changes vendor-api-surfaces` clean.
