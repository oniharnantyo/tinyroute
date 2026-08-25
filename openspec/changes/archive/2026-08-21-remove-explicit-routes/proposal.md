## Why

Explicit routes (`routes:` in config.yaml — glob-matched, surface-scoped fallback
chains for bare model names) are superseded by combos: combos provide ordered
fallback chains with pool/fused modes, capability ordering, and full management
surfaces (dashboard wizard, CLI), while routes are hand-edited YAML only — the
one routing mechanism with no management UI. Removing routes shrinks the
router's highest-complexity, least-managed resolution path and its validation,
CLI, client-installer, and dashboard surface area.

## What Changes

- **BREAKING** The router no longer resolves bare model names via explicit
  routes. Resolution order becomes: combo by name → `provider[@account]:model`
  prefix → error. Unprefixed, non-combo model names fail with a new error
  pointing at the two remaining paths (`provider:model` prefix or a defined
  combo).
- **BREAKING** `routes:` is removed from the topology config schema
  (`Topology.Routes`, the `Route` struct, and its validation block).
- **BREAKING** Existing config.yaml files containing a `routes:` block parse
  successfully but emit a deprecation warning naming the key and pointing to
  combos; the block has no routing effect. The next topology rewrite via the
  dashboard or CLI drops the block (structural rewrite reconstructs the file
  without it — intended cleanup, not data loss).
- Removed from the router: `Config`, `RouteEntry`, `ParseRoutes`, `RawRoute`,
  `parseChain`, the glob-match resolution step, and route-derived entries in
  model discovery (`Router.Models`).
- Removed from the CLI: route counts in `validate`/`status` output, route
  conversion in `serve` wiring, and route-chain mining in client-installer
  model discovery (`discoverModelsForDialect` sources combos and whitelists
  only).
- Removed from the dashboard: the Routes sidebar menu item, the
  `/dashboard/routes` page (`view_routes.templ`), and its handler. The
  sidebar re-anchors Combos between Providers and History.
- Accepted capability loss: glob bare-name matching (`match: "claude-*"`),
  per-surface scoping (`from:`), and `$model` chain interpolation disappear
  with no replacement.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `core-routing`: the resolver contract narrows — glob-routed bare names are a
  resolution error instead of resolving through a chain; model discovery no
  longer emits route-derived ids; and a new requirement covers the legacy
  `routes:` block being ignored with a warning.
- `management-dashboard`: the "Observe views render live gateway state"
  requirement drops routes from the rendered views and read-API sources; the
  "Combos section in dashboard navigation" requirement re-anchors Combos
  between Providers and History.

## Impact

- **Code:** `internal/config/topology.go` (field, struct, validation,
  deprecation warning), `internal/route/router.go` (resolution step, discovery,
  ParseRoutes family), `internal/cli/serve.go` (wiring, log line),
  `internal/cli/commands.go` (validate/status output),
  `internal/cli/clients.go` (model discovery), `internal/dashboard/`
  (`view_layout.templ` nav, `view_routes.templ` + generated Go, `handler.go`
  route + handler, `combos.go` topology round-trip).
- **Compatibility:** configs using `routes:` for bare-name routing break at
  runtime with a resolution error; the load-time warning is the only migration
  signal. No config migration tooling.
- **Specs:** `core-routing`, `management-dashboard` deltas (this change).
  `clients-install` needs no delta — its wording ("models tinyroute can route")
  already describes the post-removal behavior.
- **Docs:** `docs/ARCHITECTURE.md` and `docs/translation-architecture.md`
  mention routes and need updating.
- **Tests:** `internal/route/router_test.go` (route-path cases removed,
  new unprefixed-error cases), `internal/cli/clients_integration_test.go`,
  `internal/dialect/openai/models_test.go`, `internal/config/topology_test.go`,
  `internal/dashboard/handler_test.go`.