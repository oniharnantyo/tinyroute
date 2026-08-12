## Why

`GET /v1/models` already exists and returns an OpenAI-shaped response, but it does not honor the
spec's contract for the `id` field: *"The model identifier, which can be referenced in the API
endpoints."* `Router.Models()` today emits both the `provider:model` prefixed form **and** a bare
twin for every whitelisted model. Yet `core-routing` already mandates that bare, unprefixed models
are rejected unless an explicit manual route matches (*Requirement: Reject Unprefixed Requests*).

The result: `/v1/models` advertises IDs that `POST /v1/chat/completions` then returns `404` for.
The listing and the resolver disagree, and a client that picks a listed model can hit a dead end.
This matters because external OpenAI-compatible clients — and a future `tinyroute models` client
command — call `/v1/models` to discover what they may send. The endpoint's entire value is the
guarantee that every listed `id` is usable.

## What Changes

- **Listed IDs must be referenceable.** `Router.Models()` is constrained so every `id` it returns
  resolves successfully through `router.Resolve` on the OpenAI surface. Bare twins that no explicit
  route covers are dropped. The listing can no longer advertise a model the router will reject.
- **`created` is the stable constant `0`.** The endpoint does not track model creation time. The
  current `time.Now()` (which makes every entry's timestamp drift on each request) is replaced by
  the constant `0`.
- **`owned_by` remains the constant `"tinyroute"`.** No value change; recorded here as a conscious
  deviation from the spec's "the organization that owns the model," since tinyroute is the routing
  authority the caller actually reaches.
- **Method restricted to `GET`.** Non-GET requests to `/v1/models` return `405 Method Not Allowed`
  rather than silently returning the list.
- **Errors use the OpenAI JSON envelope.** The `500` path (router build failure) returns the same
  JSON error shape as the chat endpoint, instead of plain text.

### Non-goals (recorded so they are not relitigated)

- **Authentication is not added.** `/v1/models` stays unauthenticated so the tinyroute CLI client
  and local operators can query it without a key. This is a conscious trade against the `security.md`
  default and the spec's Bearer example, and is revisitable as a follow-on if the endpoint is ever
  exposed beyond localhost (loopback-exempt Bearer is the candidate shape then).
- **`GET /v1/models/{id}` (single-model retrieval) is not added.**
- **`created` provenance is not tracked** — no first-seen timestamps, no upstream `created` proxying.
  It is a constant by decision.
- **Cross-dialect reachability** (whether an `anthropic:` prefixed id is genuinely callable on the
  OpenAI chat surface) is out of scope; the listing invariant is defined against `Resolve`, matching
  exactly what `POST /v1/chat/completions` accepts — no more, no less.

## Capabilities

### Modified Capabilities

- `core-routing`: add the model-discovery requirement — `GET /v1/models` SHALL list only IDs that
  resolve on the OpenAI surface, with stable `created`/`owned_by` values and a `GET`-only method.

## Impact

- **Code**: `internal/route/router.go` (`Models()` — constrain to resolvable IDs), `internal/dialect/
  openai/models.go` (`created` constant, JSON error envelope), `internal/cli/serve.go` (GET-only
  guard). All surgical; no new packages.
- **Behavior change**: `/v1/models` returns a subset of its current list (bare unresolvable twins
  removed). Clients that relied on those bare IDs were already receiving `404` on use, so no working
  integration breaks.
- **Tests**: a new `internal/dialect/openai/models_test.go` asserting the invariant — every returned
  `id` resolves; `created == 0`; `owned_by == "tinyroute"`; non-GET → 405; 500 → JSON envelope.