## Why

The `antigravity` provider is scaffolded but unusable today. Its preset authenticates correctly (OAuth/PKCE already works and is specced) and advertises tiered model IDs (`gemini-3.6-flash-medium`, etc.), but it points the generic `gemini` dialect at `https://generativelanguage.googleapis.com`. Those tiered IDs exist **only** on Google's CloudCode backend (`daily-cloudcode-pa.googleapis.com/v1internal`), a different protocol surface — so every request, including dashboard model probes, returns **404**. A user completes `auth login antigravity` and still cannot route a single successful request.

Now is the right moment: the recent in-process probe routing fix lets the dashboard probe actually reach upstreams, which is what surfaced this gap. Antigravity exposes a freemium Gemini 3.x / Claude / GPT-OSS backend — high value once the runtime adapter exists. Both reference implementations (9router, OmniRoute) confirm the exact CloudCode contract, so the work is well-understood.

## What Changes

- **Add a CloudCode runtime adapter** that wraps a native Gemini payload in the CloudCode envelope `{project, model, userAgent:"antigravity", requestType:"agent", requestId, request:{…}}` and POSTs to `/v1internal:generateContent` (non-stream) or `/v1internal:streamGenerateContent?alt=sse` (stream) on `daily-cloudcode-pa.googleapis.com`.
- **Add required onboarding**: call `loadCodeAssist` (fallback `onboardUser`) to obtain the `cloudaicompanionProject`, cache it per-token, and inject it as the envelope `project`. Without this, CloudCode rejects generate calls.
- **Add IDE fingerprint headers** (`User-Agent: antigravity/ide/2.1.1 darwin/arm64` and related) required by the backend.
- **Fix the antigravity preset** to target the CloudCode transport/endpoints instead of the generic `gemini` dialect + `generativelanguage` URL. Existing OAuth credentials remain valid — no re-authentication required.
- **Tiered model dispatch**: send Antigravity model IDs to the CloudCode backend per its contract (including any required upstream-ID mapping), so the advertised whitelist is actually servable.

**Scope — minimal viable first.** This change delivers envelope + onboarding + fingerprint + preset fix: enough for requests and dashboard probes to succeed against the CloudCode backend. **Out of scope** (follow-up changes): model fallback chains, tool-cloaking/decoy-tool anti-detection hardening, the image-generation branch, and MITM model aliases. These are reliability/polish features in the reference implementations, not prerequisites for a working request.

## Capabilities

### New Capabilities

- `antigravity-cloudcode`: The CloudCode runtime adapter contract — envelope wrapping, `/v1internal:generateContent` endpoint, the required `loadCodeAssist`/`onboardUser` onboarding lifecycle (project ID cached per-token and injected), IDE fingerprint headers, tiered model dispatch, and how the antigravity preset wires to this transport instead of the generic `gemini` dialect.

### Modified Capabilities

<!-- None. The antigravity OAuth/PKCE login already works and is governed by provider-credentials
     (see its "antigravity sends its Google client_id" scenario); only the runtime transport is new.
     The design phase will confirm whether preset transport selection needs a new declarative field
     (which would touch provider-registry) or stays internal dispatch. -->

## Impact

- **Code**: new CloudCode adapter (likely `internal/dialect/antigravity/` or a translator + per-provider request-transform hook in `internal/proxy`); an onboarding component for `loadCodeAssist` state, co-located with credential resolution (`internal/credential/`); preset correction in `internal/preset/catalog.go`. The dashboard model-probe needs no change — it already reaches upstreams correctly after the routing fix.
- **Config**: `antigravity` provider entries switch to the CloudCode transport/endpoints. Stored OAuth tokens stay valid.
- **APIs**: none external; all internal.
- **Dependencies**: none new (stdlib `net/http`; OAuth refresh already implemented).
- **Risk**: CloudCode is a Google-internal API keyed to the Antigravity IDE client; the deferred anti-detection hardening (tool cloaking, decoy tools) may be required for long-term reliability but is not needed for an initial working request.
