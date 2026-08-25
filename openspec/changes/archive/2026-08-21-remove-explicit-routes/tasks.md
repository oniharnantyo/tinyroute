## 1. Config layer

- [x] 1.1 Write failing tests for `config.CheckDeprecated` (`internal/config/topology_test.go`): raw bytes containing a top-level `routes:` block return one warning naming `routes:` and mentioning combos; bytes without the key return nil; bytes with only unknown-but-unrelated keys (e.g. `future_field:`) return nil
- [x] 1.2 Implement `CheckDeprecated(data []byte) []string` in `internal/config` per design D1 (map unmarshal, top-level key scan, fixed warning copy)
- [x] 1.3 Write failing tests: `ParseTopology` on config containing `routes:` parses successfully with no error (block inert); `WriteTopology` round-trip emits no `routes:` key (design D5)
- [x] 1.4 Remove `Topology.Routes`, the `Route` struct, and the routes validation block from `internal/config/topology.go`; update `internal/config/topology_test.go` route-validation cases (unknown surface dialect, malformed hop, undeclared provider in chain) to assert the block is simply ignored

## 2. Router

- [x] 2.1 Write failing tests in `internal/route/router_test.go`: bare model name that is not a combo resolves with an error naming the model and the two supported forms, and the error text does not mention "route"; glob-style bare names (`claude-*` semantics, e.g. requesting `claude-sonnet-4-5` with no combo) fail; `Models` returns no route-derived ids
- [x] 2.2 Remove `Config`, `RouteEntry`, `ParseRoutes`, `RawRoute`, `parseChain`, the `routes` field, the glob-match resolution step, and the routes block in `Models` from `internal/route/router.go`; change `New` to `New(providers map[string]config.Provider, opts ...Option)` (design D3); replace the terminal unprefixed-model error per design D4
- [x] 2.3 Update surviving `router_test.go` cases to the new `New` signature; delete tests that existed only to cover glob-route matching and `$model` interpolation

## 3. CLI wiring

- [x] 3.1 Update `internal/cli/serve.go`: drop the `topo.Routes` → `RawRoute` conversion and the `route.ParseRoutes` call; call `route.New` with the new signature; drop the route count from the "config validated" log line (design D3)
- [x] 3.2 Add the deprecation warning to `serve` startup output and to the `validate` command: where the raw config bytes are read, call `config.CheckDeprecated` and print each warning; `validate` still exits zero when only `routes:` is present (design D1, D2)
- [x] 3.3 Update `internal/cli/commands.go`: remove route iteration from validation (around line 305) and the `routes: %d` line from `status` output (around line 484); drop the route count from the `validate` OK line (around line 265)
- [x] 3.4 Rewrite `discoverModelsForDialect` in `internal/cli/clients.go` to source provider model whitelists plus declared combo names instead of route chains (design D6); update `clients_integration_test.go` accordingly
- [x] 3.5 Remove the `Routes: rawTopo.Routes` assignment in `internal/dashboard/combos.go` (design D5); fix any other compile references to the deleted field surfaced by `go build ./...`

## 4. Dashboard

- [x] 4.1 Remove the Routes `navItem` from `internal/dashboard/view_layout.templ`; sidebar order becomes Overview, Providers, Combos, History, API Keys, Clients, Settings (design D7)
- [x] 4.2 Delete `internal/dashboard/view_routes.templ` and the generated `view_routes_templ.go`; remove the `GET /dashboard/routes` registration and `handleRoutesView` from `internal/dashboard/handler.go`; keep `icon.Route` (logo)
- [x] 4.3 Run `templ generate` (or the project's regeneration step) and update `internal/dashboard/handler_test.go`: remove routes-view cases, add a case asserting `GET /dashboard/routes` returns 404 and the sidebar renders no Routes entry (spec: "routes view is gone")

## 5. Docs and specs

- [x] 5.1 Update `docs/ARCHITECTURE.md` and `docs/translation-architecture.md`: remove explicit-routes descriptions, reflect the two-path resolution order (combo name, provider prefix)
- [x] 5.2 Update `CLAUDE.md`/README references to routes routing if any describe the `routes:` config key

## 6. Verification

- [x] 6.1 `go build ./...` and `rtk go test ./...` pass with zero route-related failures; no references to `ParseRoutes`, `RawRoute`, `Topology.Routes`, or `/dashboard/routes` remain (`rtk grep -rn "ParseRoutes\|RawRoute\|\.Routes\b\|dashboard/routes" internal/`)
- [x] 6.2 `gofmt -w .` produces no diff; `openspec validate remove-explicit-routes --strict` passes
- [x] 6.3 Manual smoke: run `go run . serve` against a config.yaml containing a `routes:` block — startup prints the deprecation warning, a bare route-matched model name fails with the D4 error, a `provider:model` request and a combo request still succeed, and `tinyroute validate` prints the warning while exiting zero