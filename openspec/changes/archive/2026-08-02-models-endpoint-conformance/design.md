## Context

`GET /v1/models` is already mounted (`internal/cli/serve.go:175`) and produces an OpenAI-shaped
list via `openai.WriteModelsResponse(router.Models())`. The shape is correct; the *semantics* are
not. Two facts are in tension today:

- `Router.Models()` (`internal/route/router.go:148`) emits **both** `provider:model` and a bare
  `model` for every whitelisted model.
- `core-routing` already mandates (*Requirement: Reject Unprefixed Requests*) that a bare model is
  rejected unless an explicit manual route matches it.

So the endpoint advertises IDs that `POST /v1/chat/completions` is spec-bound to `404`. The listing
contradicts the resolver. Additionally, `created` is `time.Now()` (drifts per request) and `500`s
are returned as plain text.

Constraints inherited from the codebase: standard library only, surgical edits, no new packages,
no `if provider == "x"` branching. `Models()` has a single production caller (`serve.go:181`).

## Goals / Non-Goals

**Goals:**

- Every `id` in the `/v1/models` response is usable in a subsequent `POST /v1/chat/completions`.
- Stable, honest field values: `created` and `owned_by` do not change between requests.
- `GET`-only method; errors in the OpenAI JSON envelope.

**Non-Goals:** (mirrors the proposal) no auth, no `GET /v1/models/{id}`, no `created` provenance,
no cross-dialect reachability guarantees.

## Decisions

### D1. Constrain the listing through `Resolve`, in the router

The invariant — *listing ⊆ resolvable* — is guaranteed by construction if `Router.Models()` filters
its candidate set through `router.Resolve(surface, id)` and keeps only the IDs that resolve.

- **Where the filter lives:** in `Router`, not in `serve.go` or the openai package. The router owns
  the contract that its own listing matches its own resolver; placing it in wiring would let the two
  drift again.
- **Surface:** `/v1/models` is the OpenAI surface, so the filter resolves against the OpenAI
  dialect's `Name()` (fetched via `dialect.ByName`, not hardcoded), so a future dialect rename needs
  no edit here.
- **Signature:** `Models()` gains a `surface string` parameter (`Models(surface string) []string`).
  It has one production caller and any test callers — all updated. This is the only breaking change.

*Alternatives considered:*
- *Emit only the `provider:model` form, never bare.* Rejected — it would drop legitimate bare names
  that **are** routable via an explicit manual route (e.g. `fast`). The spec wants resolvable IDs,
  not exclusively prefixed ones.
- *Filter in `serve.go` before calling `WriteModelsResponse`.* Rejected — scatters the invariant
  across the wiring layer; the router could still hand out a dishonest list to a future caller.

### D2. `created` is the constant `0`

tinyroute holds no model-creation provenance. `time.Now()` is actively wrong (non-stable, leaks
request time). A constant `0` is honest and trivially stable.

*Alternatives rejected:* persist a first-seen timestamp per whitelisted model (schema change,
persistence, churn on every config edit); proxy the upstream `created` via `FetchProviderModels`
(couples listing to upstream reachability and staleness). Both add mechanism for a field no client
acts on.

### D3. `owned_by` is the constant `"tinyroute"`

The caller reaches tinyroute, not the upstream org; tinyroute is the routing authority for the
listed surface. Kept as-is and recorded as a conscious deviation from the spec's prose.

### D4. `GET`-only via a ServeMux method pattern

Go 1.22+ `ServeMux` accepts a method in the pattern: `mux.HandleFunc("GET /v1/models", …)`. A
non-GET request whose path matches then returns `405 Method Not Allowed` automatically — no
hand-rolled method check. (Module is `go 1.26.4`, so this is available.)

*Alternative rejected:* an in-handler `if r.Method != GET` guard. Needlessly imperative when the mux
expresses it declaratively.

### D5. Router-build failure uses the OpenAI error envelope

Replace `http.Error(w, err.Error(), 500)` with the openai dialect's `WriteError(w, 500, "api_error",
…)`. The handler already imports the `openai` package, so this is one call-site change that makes
error shape uniform with the chat endpoint.

### D6. Authentication is deliberately not added

The endpoint stays open so the tinyroute CLI client and local operators can query it without a key.
This trades against `security.md`'s default and the spec's Bearer example, accepted because the
deployment is localhost-first. Recorded as a non-goal; the candidate shape if it must be locked down
later is **loopback-exempt Bearer** (`127.0.0.1`/`::1` skip the key check) — which would also keep
the CLI client frictionless.

## Risks / Trade-offs

- **[List shrinks — clients see fewer IDs]** → Only bare IDs that already `404`'d on use are removed.
  No working integration breaks; behavior change is purely subtractive.
- **[Unauthenticated endpoint contradicts `security.md`]** → Accepted for localhost-first use; flagged
  as a non-goal. Mitigation if exposed: D6's loopback-exempt Bearer.
- **[Listing inherits any `Resolve` bug]** → Intended. The invariant is "listing == what Resolve
  accepts," so a resolver defect propagates rather than being masked. Acceptable and observable.
- **[`created: 0` surprises a client expecting real timestamps]** → Documented deviation; no client is
  known to depend on `created` for correctness.

## Migration Plan

Single release, no data migration. All changes are in-code and subtractive. Rollback is `git revert`;
no on-disk state is touched.

## Open Questions

- Should `/v1/models` gain auth (loopback-exempt Bearer) before any non-localhost exposure? Deferred.
- Should bare names for whitelisted models ever be auto-routed (bare `gpt-4o` → its provider)? Out of
  scope — the existing "reject unprefixed" rule is left intact.