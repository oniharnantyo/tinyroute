## Context

tinyroute mounts each dialect at the upstream vendor's canonical path and uses one
string — `Dialect.Paths()` — for both the **inbound** endpoint (mounted in
`internal/cli/serve.go`) and the **outbound** upstream URL (built in
`internal/proxy/proxy.go` and two CLI test commands). The coupling is latent: the
strings are identical, so it works. `Paths()` has four callers — three outbound
(`proxy.go:133`, `commands.go:359`, `commands.go:2112`) and one inbound
(`serve.go:202`). `dialect.ByPath` is exported but never called.

The only discovery endpoint, `GET /v1/models`, is hand-mounted and OpenAI-shaped
(see archived `models-endpoint-conformance`); its invariant is "every listed `id`
resolves on the OpenAI surface." There is no Anthropic listing and no Responses
surface. `core.Translator` exists but `translate.Lookup` is a stub returning an
error for every dialect pair.

Constraints inherited from the codebase: standard library only, surgical edits,
no `if dialect == "x"` branching in the proxy, the dialect registry's promise that
"adding a dialect costs one package plus one import line — the registry and the
proxy never need to change."

## Goals / Non-Goals

**Goals**

- Each dialect is reachable under a namespaced inbound surface `/{vendor}/v1/*`,
  distinct from its outbound upstream path.
- Each surface exposes its own faithful `/models` in the vendor's native shape.
- A new `openai-responses` surface serves `POST /openai/v1/responses` end-to-end
  (non-streaming + SSE), routed by `model`.
- The router never relays a request to a provider whose dialect mismatches the
  inbound surface.

**Non-Goals**: backward-compatible aliases; cross-dialect translation; stateful
Responses features; multi-dialect providers; auth on `/models` (mirrors proposal).

## Decisions

### D1. Split `Paths()` into outbound `Paths()` + inbound `MountPaths()`

`Paths()` stays the outbound/upstream path — its three outbound callers do not
move. A new `MountPaths() []string` returns the inbound mount points
(e.g. `["/openai/v1/chat/completions"]`), consumed only by `serve.go`'s mount loop
and the registry indexing. This is the only interface change that touches existing
callers, and it touches one. The stale doc comment on `Paths()` ("handles inbound")
is corrected.

*Alternatives rejected:* derive inbound as `"/"+name+path` (couples naming, breaks
when a URL prefix is shared across distinct dialects — see D6); rename `Paths()` →
`UpstreamPaths()` and repurpose `Paths()` as inbound (churns three outbound callers
for no gain).

### D2. No backward compatibility — legacy `/v1/*` removed

The value of a multi-vendor gateway is that the surface is explicit in the URL.
Keeping `/v1/*` aliases defeats that and leaves two sources of truth per surface.
Internal blast radius is contained: one production mount line (`serve.go`) plus six
`*_test.go` files whose request URLs update mechanically. Critically, every `/v1/*`
reference that survives is an **outbound** path — `FetchProviderModels`
(`config/catalog.go`) and the dialect `Paths()` — and those *must* stay canonical,
since they talk to real upstreams. Only inbound wiring moves.

*Alternative rejected:* mount both namespaced and legacy paths. Doubles the mount
set per dialect and leaves the surface ambiguous.

### D3. Resolution is faithful to the inbound surface

`Router.Resolve(surface, model)` rejects a resolved hop whose provider dialect ≠ the
inbound surface's dialect. Without a translator, a cross-dialect hop would relay a
body in the wrong shape to a provider that returns 400 — silent breakage. Rejecting
it makes the surface mean what it says. The guard lives in the router, not the
proxy, so the proxy stays dialect-agnostic (per D14).

*Alternative rejected:* allow cross-dialect hops and rely on the (stubbed)
translator. The translator is not built; see Future.

### D4. Per-surface `/models` via `WriteModels` on the interface

The two vendors' list shapes are not isomorphic (OpenAI: `object`/`created`(unix);
Anthropic: `type`/`display_name`/`created_at`(RFC3339)/capabilities/pagination).
One writer cannot serve both without branching. `core.Dialect` gains
`WriteModels(w, ids []string)`, mirroring the existing `WriteError`/`AuthHeaders`
pattern (the dialect owns its wire format). `serve.go` mounts
`GET {mountBase}/models` generically per dialect and calls `d.WriteModels`.

Listing *content* is already surface-correct for free: `Router.Models(surface)`
returns only ids that resolve on that surface, so under D3 each `/models` lists only
its own dialect's models.

### D5. Anthropic `/models` data is honest-minimal (P1)

tinyroute holds only a model-name whitelist per provider — no `display_name`,
`created_at`, `max_tokens`, or `capabilities`. Under P1 the Anthropic writer emits
honest constants: `type:"model"`, `display_name` echoes the `id` (no fabrication),
`created_at` is the epoch (`1970-01-01T00:00:00Z` — the Anthropic spec explicitly
sanctions an epoch when the release date is unknown, mirroring the OpenAI
`created: 0` decision), `max_input_tokens`/`max_tokens` are omitted, capabilities are
absent or `{supported:false}`, `has_more:false`. Pagination params (`after_id`/
`before_id`/`limit`) are accepted and ignored — the whitelist is small.

*Alternative P2 (upstream mirror)* — fetch the real upstream `/v1/models` and relay
rich fields. Rejected for the same reason archived D2 rejected upstream-proxied
`created`: it couples the listing to upstream reachability and staleness, and adds a
cache. If real capability data is later required, P2 is the path and becomes its own
change.

### D6. `openai-responses` is a separate dialect, not a second path on `openai`

The proxy builds the outbound URL from `hopDialect.Paths()[0]` — a dialect is
effectively single-outbound-path in practice. `/v1/chat/completions` and
`/v1/responses` are different upstream endpoints, and the two protocols need
different `ParseRequest` (`input`/`instructions` vs `messages`), different usage
scanners (`response.usage` in the `response.completed` SSE event vs `usage` in the
final `data:` chunk), and a real vs no-op `InjectUsageOption`. Cramming both into
`openai` means `if path == "/responses"` branching — forbidden by convention. The
`core.Dialect` doc comment already names `openai-responses` as a future dialect.
Adding it is one package + one import line; `serve.go`'s registry-driven mount loop
picks it up with zero changes (validating the "proxy never changes" promise).

Mount path `/openai/v1/responses` shares the `/openai/` prefix with
`/openai/v1/chat/completions` but the dialect surface name is `openai-responses` —
confirming D1's choice of explicit `MountPaths()` over name-derived paths.

### D7. Responses is a thin pass-through; stateful features are non-goals

The Responses API is stateful (`store`, `previous_response_id`, `conversation`,
`background`, built-in tools). tinyroute is a stateless router. tinyroute routes by
`model`, rewrites the model, relays the response (including SSE), and extracts usage
from `response.completed`. It does not store responses, honor `previous_response_id`
across requests, run built-in tools, or expose the background/poll endpoint.
Failover is honored only before first byte (the existing window); a multi-turn
conversation pinned via `previous_response_id` is stable only while its provider
stays healthy. Recorded as non-goals, not limitations to fix.

### D8. One dialect per provider config entry

`config.Provider.Dialect` is singular; the proxy builds the outbound URL/headers
from it. To expose one upstream OpenAI account on both `/openai/v1/chat/completions`
and `/openai/v1/responses`, configure two providers (same base URL + key, different
`dialect`, each whitelisting its models). Under D3 the model prefix then differs by
surface (`openai:gpt-5.1` vs `openai-responses:gpt-5.1`). Redundant but simple.

*Alternative rejected:* a multi-dialect provider (`Dialect []string` or a dialect
family). A config-schema + router change; deferred until the duplication bites.

### D9. `core.Usage` gains `ReasoningTokens`

Responses usage reports `output_tokens_details.reasoning_tokens`, a dimension
chat-completions does not expose. Adding one field to `core.Usage` lets the
responses usage scanner capture it faithfully and benefits future reasoning models.
The OpenAI chat scanner leaves it `0`. Small, additive.

*Alternative rejected:* drop reasoning tokens silently. Loses data a usage/billing
surface may want; not worth the saving.

## Risks / Trade-offs

- **[Paths move, no compat]** → Accepted (D2). Mitigated: nothing external depends on
  the old paths yet; outbound paths untouched; internal updates are mechanical.
- **[Anthropic `/models` is capability-poor under P1]** → Accepted (D5). A client
  reading `capabilities.thinking.supported` to gate a request gets a non-answer.
  Documented; P2 is the upgrade path if needed.
- **[Responses stateful features unsupported]** → Accepted (D7). Non-goal; clients
  relying on `store`/`previous_response_id` get single-turn pass-through only.
- **[Two-provider config redundancy for chat+responses]** → Accepted (D8).
- **[Model prefix differs per surface]** → `openai:gpt-5.1` vs
  `openai-responses:gpt-5.1` is slightly awkward but unambiguous and consistent with
  the `provider:model` rule.

## Migration Plan

Single release, no data migration. All changes are in-code. Path moves are
subtractive on the inbound side only. Rollback is `git revert`; no on-disk state is
touched. Operators must update any client `base_url` from `{host}/v1` to
`{host}/{vendor}/v1` and any `model` reference to the matching surface's prefix.

## Open Questions

- Should `ReasoningTokens` propagate into the history/usage records, or stay
  internal to the in-memory `Usage`? (Lean: propagate, for parity with other fields.)

## Future: universal cross-dialect translation

The `core.Translator` stub stays. The intended approach — deferred to a separate,
larger change — is the **OpenAI-as-canonical-hub** pattern, validated by the
vendored reference at `9router/open-sse/translator/`:

- Every translation pivots through OpenAI Chat Completions as the interchange format
  (no invented canonical type): request `source → openai → target`; response
  `target → openai → source`.
- Translators self-register as `register(from, to, reqFn, resFn)` pairs — **both**
  request and response modeled (the current stub models request only and is
  insufficient; response translation, especially streaming, is mandatory).
- Streaming response translation is a per-stream state machine (`initState`).
- **Direct routes** for fragile pairs (e.g. `claude→kiro`) skip the hub to avoid the
  lossy double-hop.
- Shared **`concerns/`** modules (`toolCall`, `thinking`, `usage`, `image`, `json`)
  factor out the cross-pair knowledge that pure pairwise translators would duplicate.

If pursued: stage text-only first (messages + system + model + stream +
`max_tokens`); defer tools/multimodal/reasoning; reject cross-dialect *streaming*
initially. Effort is substantial — `open-sse/translator/` is ~48 files — and
constitutes a different product thesis (universal translator gateway) than the
faithful router this change ships.
