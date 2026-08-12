## Context

The dashboard provider-detail page (`internal/dashboard/handler.go` + `view_provider_detail.templ`) currently treats **"is this provider in the topology?"** as a hard gate. A preset that has not been "Configure"d is absent from `rawTopo.Providers`, and four mutation handlers respond `"Provider not found"` when targeted:

- `handleProviderCredential` (save static key) — `handler.go:895`
- `handleModelAdd` (whitelist a model) — `handler.go:986`
- (indirectly) `handleOAuthCallback` — stores a token via the credential store but leaves the provider non-routable because it is never added to the topology

Only `handleProviderAdd` (the **Configure** button) writes the topology entry, and it writes only a `${VAR}` placeholder — no usable secret. The UI nonetheless renders the API-key and OAuth forms for *every* provider, so they look usable while being blocked. `handleOAuthStart` already skips the topology check, so OAuth *half*-works on unconfigured presets — an inconsistency.

Routing is untouched by this design: the proxy resolves credentials and routes purely from the topology, which the `TopologyWatcher` reloads on file write. That invariant is what makes "write the topology lazily" safe.

## Goals / Non-Goals

**Goals:**
- Connecting to a provider (static key, OAuth, or whitelisting a model) SHALL be a single action that works on any available preset, with no prerequisite "Configure" step.
- Collapse the `Configured` boolean gate into derived, user-meaningful state (available / activated / connected).
- Remove all four `"Provider not found"` dead-ends behind one shared, idempotent materialization seam.
- Preserve the topology file format, `${VAR}` references, atomic `0600` writes, and the routing/proxy layer exactly as-is.

**Non-Goals:**
- Relocating the static API key from plaintext-in-topology into the credential store (deferred — see Decision 6).
- Changing preset definitions, the credential store, the CLI (`provider add` / `auth login`), or the proxy read path.
- Changing the providers *list* grouping/sections (only the detail page and per-card status text are in scope).

## Decisions

### Decision 1 — Lazy materialization through one `ensureMaterialized(provName)` helper
Every mutation handler calls a shared helper at the top: if `provName` is a known preset **and** not already in `rawTopo.Providers`, write a topology entry materialized from the preset defaults (reusing the logic currently in `handleProviderAdd:810-824`), then proceed with the mutation.

- *Why one helper:* the four dead-ends are the same bug in four places. Funneling through one seam makes the invariant — "any mutation target exists in the topology before it is mutated" — impossible to violate per-handler.
- *Alternative considered:* inline the materialization in each handler. Rejected — drift between handlers is exactly the inconsistency we are removing (OAuth already diverges today).
- *Alternative considered:* collapse topology membership entirely and route from presets ∪ topology at request time. Rejected — it would push preset-resolution into the hot proxy path and change routing semantics. Keeping topology as the single routing source-of-truth and writing it lazily gets the UX win without touching routing.

### Decision 2 — Replace the `Configured` boolean with a three-state model
The detail handler derives, instead of a single bool:

| State | Meaning | Drives |
|---|---|---|
| **Available** | in topology **or** is a known preset | whether the detail page exists at all |
| **Activated** | in topology | whether it is customized / will persist |
| **Connected** | has a credential (static key or OAuth record) | whether it can actually route; the status badge |

`Configured` ceases to be a gate; it becomes one input to the derived **Connected** status.

- *Why:* the boolean conflates "the user did something" with "the provider can serve traffic." Separating them lets the UI say precisely what is wrong (e.g. *Awaiting credentials* — activated but not connected) instead of failing silently at request time.

### Decision 3 — Wide materialization triggers; Models section always renders
Any mutation (static key, OAuth callback, model whitelist) materializes. The Models section renders for every available provider, not only configured ones (catalog already loads by preset name — `handler.go:521-525`).

- *Why:* this is the true "collapse." A preset with whitelisted models but no credential lands in the topology; routing for it fails at request time, which we surface explicitly via the *Awaiting credentials* status (Decision 2) rather than hiding the Models section.
- *Alternative considered:* narrow triggers (materialize only on credential save; keep Models gated). Rejected — it preserves the very two-step mental model the user asked to remove, and it re-creates a gate (whitelist needs activation first).

### Decision 4 — Remove the Configure banner and the prominent Configure button
The amber "Provider Not Configured" banner and its **Configure** action are removed. The Connections section becomes the primary surface on every detail page.

- *Why:* the user's explicit ask — "when open the provider detail user can directly do connection." Connection *is* configuration.
- *Env-var-only activation path:* a preset that should use `${VAR}` without a pasted key is activated implicitly the first time the user whitelists a model (Decision 3). If that proves insufficient, a quiet secondary "Activate" control can be added later — out of scope here.

### Decision 5 — Deleting a preset-backed provider reverts it to a clean preset
`handleProviderDelete`, when the provider is preset-backed, SHALL remove the topology entry **and** any stored credentials, then return the user to the list where the preset reappears as an available, unconnected card. Custom (non-preset) providers are removed entirely.

- *Why:* an emergent, better mental model. "Configure to add / delete to remove" becomes "connect to use / delete to reset." Presets are always re-available, so delete is non-destructive to discoverability.

### Decision 6 — DEFER relocating the static key into the credential store
This change preserves current behavior: a pasted static key is stored as `prov.APIKey` in the topology (plaintext), exactly as today. Moving it to the credential store (keeping only a reference in topology) aligns with `security.md` but touches the proxy's credential-resolution read path and is a behavior change unto itself.

- *Why defer:* keeps this change at behavioral parity for storage, minimizing blast radius and review surface. Lazy-materialize is the natural seam for the future relocation, which becomes a follow-up change scoped to the read path.

## Risks / Trade-offs

- **[Materialization inconsistency across handlers]** → Mitigation: the single `ensureMaterialized` helper (Decision 1); one test per mutation handler asserting a preset-not-in-topology succeeds.
- **[Activated-but-uncredentialed provider looks routable but fails at request time]** → Mitigation: the explicit *Awaiting credentials* status (Decision 2); the existing auth-cooldown / health signals already classify missing-credential failures. A provider without a credential resolver cannot serve traffic regardless of topology membership.
- **[OAuth callback writes a token before the topology entry exists]** → Mitigation: materialize idempotently *before* / within the credential write in `handleOAuthCallback`; a second materialize call is a no-op (entry already present).
- **[Test suite asserts the old "must configure first" ordering]** → Mitigation: `TestProviderCRUDAndCredentials` and neighbors are rewritten as expectation flips (save-on-unconfigured now succeeds), not just extended.
- **[Wide triggers can surprise users who browse presets]** → Mitigation: browsing happens on the list; the detail page is where intentional actions occur. Materialization only follows an explicit mutation.

## Migration Plan

- **No data/config migration.** Topology file format and `${VAR}` references are unchanged. Already-configured providers are unaffected (they are already in the topology; helpers are no-ops for them).
- **Rollback:** revert the code; no on-disk state needs undoing.
- **Deploy:** single build of the dashboard handler + template. Routing/proxy unchanged.

## Open Questions

- Should an explicit "Activate without credentials" secondary control exist for env-var-only presets, or is "whitelist a model" a sufficient implicit activation path? (Lean: sufficient for now; revisit if requested.)
- Should the providers-*list* card badge also become connection-derived (today it shows "Not configured" vs a connection count), or only the detail-page header?
- Exact copy/placement of the *Awaiting credentials* state on the detail page.
