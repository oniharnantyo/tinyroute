## Context

tinyroute proxies inference via a dialect-centric path: resolve route → pick hop dialect → translate the body if inbound≠hop → `RewriteModel` → `POST JoinURL(BaseURL, dialect.Paths()[0])` (`internal/proxy/proxy.go` ~line 180–265). The `antigravity` preset sets `dialect: "gemini"` + `base_url: generativelanguage.googleapis.com`, so requests hit the generic Gemini API, which **404s** the CloudCode-only tiered model IDs. OAuth/PKCE login already works and is governed by `provider-credentials`.

The gap is purely runtime. CloudCode needs: (a) a request **envelope** nesting the native Gemini payload under `request`, (b) a different endpoint (`/v1internal:generateContent`), (c) a **required onboarding step** that yields a `cloudaicompanionProject` injected as `project`, and (d) IDE headers. None of this fits the dialect contract, which is *stateless* — dialects are pure transforms of (body, model) with no access to context or credential state, so they cannot resolve or inject the onboarding project ID (resolved via network I/O that must run *before* body assembly).

## Goals / Non-Goals

**Goals:**
- Make antigravity requests and dashboard probes succeed against the CloudCode backend.
- Isolate CloudCode complexity so the existing dialect-driven path (every other provider) is untouched and zero-risk.
- Reuse the working OAuth flow and the gemini response handling.

**Non-Goals:**
- Anti-detection hardening (tool cloaking, decoy tools, sophisticated `requestId`) — follow-up.
- Model fallback chains, image-generation branch, MITM model aliases — follow-up.
- Persisted onboarding state — in-memory cache first.
- Generalizing a transport framework beyond `cloudcode` — build only what's needed.

## Decisions

**D1 — Introduce a per-hop transport hook; CloudCode is the first non-default transport.**
At the top of the per-hop loop (`internal/proxy/proxy.go`), if the provider's `transport` is `"cloudcode"`, delegate the entire send to a CloudCode executor; otherwise the existing dialect path runs unchanged. *Rationale:* the dialect contract can't obtain/inject the project ID, and the envelope+endpoint are a genuinely different protocol. A dedicated executor (the 9router/OmniRoute model) is the honest fit. *Alternative rejected:* implement as a new `antigravity` dialect — dialects are pure transforms with no context/credential access, so they cannot run onboarding at `RewriteModel` time (which precedes credential resolution).

**D2 — Declare transport via an optional provider field `transport` (default empty).**
Empty/absent = standard dialect path (all current providers unaffected). The antigravity preset sets `transport: "cloudcode"` + the CloudCode base_url. *Rationale:* explicit and extensible, parallel to the existing non-standard-auth `flow_type` pattern in `provider-registry`. *Alternative rejected:* implicit dispatch by provider name — magic, unextensible.

**D3 — Onboarding lives in a new `internal/cloudcode` package with an in-memory per-token cache (~1h TTL).**
`Onboarding.ProjectID(ctx, accessToken) (string, error)` returns a cached value or calls `loadCodeAssist` (fallback `onboardUser`) at `cloudcode-pa.googleapis.com/v1internal:...`, caches by token, and returns the `cloudaicompanionProject`. *Rationale:* onboarding is credential/token-adjacent (like OAuth refresh) but is provider-protocol state, not a token or secret — so it sits in its own package, not in `internal/credential` or config. In-memory first (re-onboard on restart is one cheap call); persistence deferred. *Alternatives rejected:* store in the credential store (not a token), store in config (runtime-discovered, not user-configured).

**D4 — The executor wraps a native Gemini payload; response handling reuses the gemini dialect.**
The executor takes the already-translated native Gemini body (same inbound→gemini translation as today), wraps it as `{project, model, userAgent:"antigravity", requestType:"agent", requestId, request:{<body>}}`, POSTs to `/v1internal:generateContent` (or `:streamGenerateContent?alt=sse`), sets IDE headers + Bearer token, and relays the response. The response is native Gemini (confirmed by both references), so usage scanning and response translation reuse the gemini dialect unchanged. Only the request envelope + endpoint are CloudCode-specific.

**D5 — IDE fingerprint is minimal; hardening deferred.**
Send `User-Agent: antigravity/ide/2.1.1 darwin/arm64` + `Authorization: Bearer`. `requestId` is a generated value. Tool cloaking / decoy tools are a Non-Goal.

**D6 — The dashboard probe needs no change.**
The probe (`internal/probe/probe.go` `RunInProcess` → `proxy.Handler`) resolves `antigravity:<model>` and drives the handler; with D1 the handler delegates to the CloudCode executor, so a probe exercises onboarding+envelope and reports real upstream status. The probe body is built with the gemini dialect (native payload), which the executor wraps.

## Risks / Trade-offs

- **[CloudCode is a Google-internal API — may change; fingerprint requirements may shift]** → Mitigation: isolate in `internal/cloudcode`; keep endpoints/headers/version configurable; the executor is the single seam, so hardening lands later without rework.
- **[Onboarding adds first-request latency]** → Mitigation: per-token cache; optionally warm on `auth login` or first probe.
- **[Transport hook branches the proxy hot path]** → Mitigation: default path unchanged; one early check, zero overhead for non-cloudcode providers.
- **[No anti-detection → possible throttle/rejection under sustained use]** → Mitigation: documented Non-Goal; probe/light use works initially.
- **[Model-ID form uncertain — verbatim vs `tiered(...)` mapping]** → Mitigation: verify against the live API during implementation; keep any mapping in preset data, not code.

## Migration Plan

- Preset fix applies automatically to newly-added antigravity providers.
- Existing user configs with an `antigravity` entry (`dialect: gemini`, `base_url: generativelanguage`) need `transport: "cloudcode"` + corrected base_url. Recommended: a one-time load-time migration that sets these for a provider named `antigravity` lacking `transport` (finalize in tasks); fallback is re-add.
- No credential migration — OAuth tokens remain valid.
- Rollback: revert the change; antigravity returns to 404 (no worse than today). No destructive operations.

## Open Questions

- Model-ID form: send tiered IDs verbatim or map to `gemini-3.6-flash-tiered(medium)`? Verify live.
- Is the generate response always unwrapped native Gemini (stream + non-stream)?
- Persist the onboarding project ID across restarts, or in-memory only (current proposal)?
- Migration: auto-migrate existing antigravity config entries, or require re-add?
