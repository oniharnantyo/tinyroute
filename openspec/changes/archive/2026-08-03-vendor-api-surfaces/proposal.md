## Why

tinyroute today mounts every dialect at the upstream vendor's canonical path:
`/v1/chat/completions` (OpenAI), `/v1/messages` (Anthropic), and a single
OpenAI-shaped `GET /v1/models`. As a multi-vendor gateway this is ambiguous and
limiting. A single base URL cannot host both an OpenAI-shaped and an
Anthropic-shaped surface without the path itself disambiguating the wire format;
`/v1/models` is the only discovery endpoint and it is OpenAI-shaped only; and
there is no Responses surface at all. Clients must know each vendor's path layout
rather than addressing tinyroute by *surface*.

The current design also has a latent smell: each dialect's `Paths()` is used for
**two** jobs — mounting the inbound endpoint in `serve.go` *and* building the
outbound upstream URL in `proxy.go`. That coupling is invisible only because the
two strings happen to be identical today; namespacing the inbound path breaks it.

## What Changes

- **Namespaced vendor surfaces.** Each dialect mounts its inbound endpoints under
  `/{vendor}/v1/*`: `/openai/v1/chat/completions`, `/anthropic/v1/messages`. The
  outbound upstream paths (`Paths()`) are unchanged — they remain the real vendor
  API paths.
- **Split the dual-purpose `Paths()`.** `core.Dialect` keeps `Paths()` as the
  outbound/upstream path (callers: `proxy.go` + two CLI commands, untouched) and
  gains `MountPaths()` for the inbound mount (caller: `serve.go`). `dialect.ByPath`
  is exported but unused and is dropped.
- **No backward compatibility.** The legacy un-namespaced paths (`/v1/chat/completions`,
  `/v1/messages`, `/v1/models`) are removed. The surface is explicit in the URL.
- **Faithful surfaces.** The router resolves only same-dialect providers for a
  given inbound surface; a cross-dialect hop is rejected with a clear error rather
  than relayed to a provider that would reject the body shape. (Cross-dialect
  translation is a recorded future direction, not in scope — see `design.md`.)
- **Per-surface model discovery.** `GET /v1/models` is relocated to
  `GET /openai/v1/models`, and a new `GET /anthropic/v1/models` is added. Each is
  rendered in that vendor's native list shape via a new `WriteModels` method on
  `core.Dialect`.
- **OpenAI Responses surface.** A new `openai-responses` dialect serves
  `POST /openai/v1/responses` as a thin pass-through (routes by `model`, rewrites
  model, relays SSE, extracts usage from `response.completed`).

### Non-goals (recorded so they are not relitigated)

- **Backward-compatible `/v1/*` aliases are not provided.** A conscious break.
- **Cross-dialect translation is not built.** The `core.Translator` stub stays;
  the intended approach (OpenAI-as-canonical-hub, mirroring `9router/open-sse`) is
  recorded in `design.md` as a future change.
- **Stateful Responses features are not provided** (`store`, `previous_response_id`,
  `conversation`, `background`, built-in tools). `/openai/v1/responses` is a thin
  pass-through; behavior across failover of server-side state is not guaranteed.
- **Multi-dialect providers are not introduced.** A `config.Provider` keeps a single
  `Dialect`; exposing one upstream account on two surfaces means two provider entries.
- **Authentication on `/models` is not added** (unchanged from the prior
  `models-endpoint-conformance` decision: localhost-first, unauthenticated).

## Capabilities

### Modified Capabilities

- `core-routing`: namespace inbound endpoints per dialect; make resolution faithful
  to the inbound surface; relocate/per-surface model discovery; add the Responses surface.

## Impact

- **Code**: `internal/core/interfaces.go` (`Dialect` gains `MountPaths()` +
  `WriteModels()`; `Usage` gains `ReasoningTokens`); `internal/route/router.go`
  (`Resolve` faithful-surface guard); `internal/cli/serve.go` (mount `MountPaths()`,
  per-surface `/models`); `internal/dialect/{openai,anthropic}` (implement the new
  methods); new `internal/dialect/openairesponses` package. All surgical except the
  new dialect, which is additive. The proxy (`internal/proxy`) is unchanged.
- **Behavior change**: inbound paths move; `/v1/*` returns 404. Internal blast radius
  is 1 production mount line + 6 `*_test.go` files whose request URLs must update.
  Outbound paths (`config/catalog.go` `FetchProviderModels`, dialect `Paths()`) are
  untouched — they talk to real upstreams.
- **Spec consequence**: the prior `models-endpoint-conformance` requirement (written
  against `GET /v1/models`) is amended to the relocated, per-surface form.
- **Tests**: new `openairesponses` dialect tests; updated `openai`/`anthropic` model
  tests; `route` tests for the faithful-surface guard; path-string updates across the
  6 existing test files.