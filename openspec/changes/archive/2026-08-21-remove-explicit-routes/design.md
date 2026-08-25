## Context

See proposal.md for motivation. Relevant current state:

- `Router.Resolve` (`internal/route/router.go:118`) resolves in three steps:
  combo by name → `provider[@account]:model` prefix → glob-matched explicit
  routes. The routes step and its supporting types (`Config`, `RouteEntry`,
  `ParseRoutes`, `RawRoute`, `parseChain`) are the removal target.
- `Topology.Routes` (`internal/config/topology.go:51`) feeds `serve` wiring
  (`internal/cli/serve.go:158`), CLI output (`commands.go:265,305,484`),
  client-installer model discovery (`clients.go:188`), the dashboard routes
  view, and the dashboard combos write path (`combos.go:452`).
- YAML parsing intentionally tolerates unknown fields (`topology.go:101`) so
  config.yaml can carry future/unmodeled fields. `ParseRawTopology` exists for
  edit paths that preserve `${VAR}` references; writers re-marshal the struct.

## Goals / Non-Goals

**Goals:**

- Resolver and model discovery behave identically with and without a
  `routes:` block present (the block is inert).
- Deprecation warning observable at `serve` startup and in
  `tinyroute validate` output.
- Removal is complete: no dead types, no orphaned handlers, no spec text
  referencing routes.

**Non-Goals:**

- No migration tooling for configs (no converter from routes to combos).
- No attempt to preserve glob bare-name matching, `from:` scoping, or
  `$model` interpolation elsewhere (accepted loss, per proposal).
- No changes to combo behavior, prefix routing, or translation.

## Decisions

### D1: Detect `routes:` with a raw structural check, not strict parsing

Turning on `yaml.Decoder.KnownFields(true)` would reject every unmodeled
field, breaking the documented forward-compat contract. Instead, add a pure
function in `internal/config`:

```go
// CheckDeprecated scans raw config bytes for keys whose feature has been
// removed and returns human-readable deprecation warnings.
func CheckDeprecated(data []byte) []string
```

It unmarshals into `map[string]any` (cheap, one-shot, ignore error — a file
that fails strict key checking here has already passed `ParseTopology`), sees
a top-level `routes` key, and returns a fixed warning naming the key and
pointing to combos. Callers: `serve` startup and the `validate` command, at
the points where the file bytes are already in hand. The topology watcher is
not plumbed — the spec only requires startup and validate observability.

*Alternative rejected:* emitting the warning from inside `ParseTopology`
(would require changing its signature or logging from library code, and
would re-warn on every hot-reload tick).

### D2: `validate` stays exit-zero on `routes:`

The warn-on-load posture means the warning is informational. `validate`
prints the warning and still reports OK; it fails only on real topology
errors. A hard failure here would contradict the accepted back-compat
choice.

### D3: Router constructor loses the routes parameter

`route.New(routes []RouteEntry, providers, opts...)` becomes
`route.New(providers, opts...)`. `RouteEntry`, `ParseRoutes`, `RawRoute`,
`parseChain`, and the `routes` field are deleted outright rather than
deprecated — the package is internal, there is no external consumer to
migrate. `serve.go` drops the `topo.Routes` → `RawRoute` conversion.
`Router.Models` drops its routes block; listed ids continue to come from
combos and provider whitelists.

### D4: Resolution error names the two remaining paths

The terminal error at the end of `Resolve` changes from
`unprefixed model %q requires explicit route configuration` to wording of the
form:

```
model %q is not a combo and has no provider prefix — use "provider:model" or define a combo
```

The spec scenario asserts the error does not reference route configuration;
wording must survive that check.

### D5: Structural rewrite drops the block — intended cleanup

Dashboard/CLI writers re-marshal `Topology`; once `Routes` is removed from
the struct, any rewrite emits config.yaml without `routes:`. Combined with
D1's warning this is the deliberate two-phase deprecation: warning for
awareness, rewrite for cleanup. No writer code attempts to preserve the
block. The `combos.go:452` round-trip assignment is simply deleted.

### D6: Client-installer discovery sources whitelists and combos

`discoverModelsForDialect` (`internal/cli/clients.go:188`) loses its
route-chain mining. Replacement source: provider model whitelists for the
dialect plus declared combo names — the same population `Router.Models`
exposes, keeping the installer and model discovery in agreement.

### D7: Dashboard removal is a clean delete

Remove the nav item (`view_layout.templ:56`), `view_routes.templ` and its
generated `view_routes_templ.go`, the `GET /dashboard/routes` registration
(`handler.go:105`), and `handleRoutesView`. `icon.Route` stays — it is the
product logo in the sidebar header (`view_layout.templ:30,45`). Post-order
sidebar: Overview, Providers, Combos, History, API Keys, Clients, Settings.

## Risks / Trade-offs

- [Bare-name requests that worked via routes start failing] → D1 warning at
  startup/validate plus D4's actionable error message; release notes should
  call out the removal.
- [Model discovery and client model options shrink] → correct by definition
  of the removal; spec scenarios updated to the narrower contract.
- [Rewrite silently deletes user YAML] → spec'd as intended behavior
  (core-routing delta, "Topology rewrite drops the block"); the warning fires
  before any rewrite can happen.
- [Signature change breaks `route.New` callers/tests] → mechanical update;
  all callers are internal and enumerated in the proposal's Impact list.
- [`CheckDeprecated` map unmarshal cost] → negligible: once per `serve` start
  and per `validate` run, not per request.

## Migration Plan

Single-release removal, no staged rollout (single-user gateway tool).

1. Ship the removal with the deprecation warning.
2. Rollback, if needed, is `git revert` of the change. Caveat: `WriteTopology`
   writes tmp+rename with no backup, so a config rewritten without `routes:`
   under the new code loses that block permanently — users who need it back
   restore it from their own backups. This is accepted under D5.