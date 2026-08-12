TDD-ordered: write the invariant test first (RED), make it pass (GREEN), then verify. Standard
library only; no new packages; surgical edits to `route`, `dialect/openai`, and `cli`.

## 1. Test the invariant first (RED)

- [ ] 1.1 Create `internal/dialect/openai/models_test.go`. Build a topology with a whitelist-only
      provider (e.g. `openai` → `gpt-4o`) and an explicit bare manual route on the OpenAI surface
      (e.g. match `fast`). Drive the handler and assert: `openai:gpt-4o` is present, the bare
      `gpt-4o` is **absent**, and `fast` is present.
- [ ] 1.2 Assert every returned `id` resolves: for each `id` in `data`, `router.Resolve("openai", id)`
      returns no error. (This is the load-bearing assertion — the spec's "can be referenced" clause.)
- [ ] 1.3 Assert every entry has `created == 0` and `owned_by == "tinyroute"` across two requests.
- [ ] 1.4 Assert `POST /v1/models` → `405 Method Not Allowed`.
- [ ] 1.5 Assert a router-build failure yields a JSON body with an `error` object and
      `Content-Type: application/json` (not plain text).
- [ ] 1.6 Run `go test ./internal/dialect/openai/... ./internal/route/...` — confirm RED (bare twin
      present, `created` non-zero, POST returns 200, 500 is plain text).

## 2. Constrain the listing through Resolve (GREEN)

- [ ] 2.1 Change `Router.Models()` → `Models(surface string) []string` in `internal/route/router.go`.
      Keep the existing candidate generation, then retain only IDs for which `r.Resolve(surface, id)`
      succeeds. Preserve the `seen`-map dedup.
- [ ] 2.2 Update the single production caller in `internal/cli/serve.go` to pass the OpenAI dialect's
      `Name()` (resolve via `dialect.ByName("openai")`, do not hardcode the string).
- [ ] 2.3 Update any `Models()` callers in `internal/route/router_test.go` to the new signature; add
      a direct unit test there that an unprefixed whitelist-only model is filtered out while an
      explicitly-routed bare name is kept.
- [ ] 2.4 (Optional, not required by spec) sort the returned slice for stable output, and note in a
      comment that ordering is not contractual.

## 3. Stable fields (GREEN)

- [ ] 3.1 In `internal/dialect/openai/models.go`, set `Created: 0` in `WriteModelsResponse` instead of
      `time.Now().Unix()`; drop the now-unused `time` import.
- [ ] 3.2 Confirm `OwnedBy` remains `"tinyroute"` (no edit) and that no other field regressed.

## 4. Method restriction and error envelope (GREEN)

- [ ] 4.1 In `internal/cli/serve.go`, change the registration to `mux.HandleFunc("GET /v1/models", …)`
      (Go 1.22 method pattern) so non-GET → `405` automatically.
- [ ] 4.2 In the same handler, replace the `http.Error(...)` on router-build failure with the openai
      dialect's `WriteError(w, http.StatusInternalServerError, "api_error", msg)`.

## 5. Verify and accept

- [ ] 5.1 `go test ./...` — all green; coverage for the openai package and route package meets the
      80% bar for the touched code.
- [ ] 5.2 `gofmt -w .` and `go vet ./...` clean.
- [ ] 5.3 Manual smoke: `go run . serve`, then
      `curl -s localhost:8787/v1/models | jq` (no bare unresolvable twins; `created` is 0;
      `owned_by` is `tinyroute`) and `curl -s -X POST localhost:8787/v1/models -i` → `405`.
- [ ] 5.4 Cross-check: each scenario in `specs/core-routing/spec.md` maps to a passing test
      (resolvable-only list, bare-route inclusion, stable fields, 405, JSON error).
- [ ] 5.5 `openspec validate --changes models-endpoint-conformance` clean.