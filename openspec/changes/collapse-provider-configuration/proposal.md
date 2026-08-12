## Why

The provider detail page forces a two-step journey: open an unconfigured preset → press **Configure** (which only drops a `${VAR}` placeholder into the topology) → *then* the connection forms actually work. The friction is worse than it looks — the page renders the API-key and OAuth forms for *every* provider, so they look actionable, but saving a static key or whitelisting a model silently dead-ends with `"Provider not found"` until Configure is pressed first. OAuth already half-works on unconfigured presets (it doesn't check topology membership), making the behavior inconsistent across credential types. Connecting a provider should *be* the act of configuring it — one action, not two.

## What Changes

- **Remove the Configure gate.** The standalone **Configure** action and the amber "Provider Not Configured" banner are removed from the provider detail page. Connection becomes the primary action.
- **Connection actions work directly on available presets.** Saving a static API key, completing an OAuth flow, and whitelisting a model SHALL each succeed for any available provider (a preset not yet in the topology) by **lazily materializing** the preset into the topology first, then applying the mutation. The four scattered `"Provider not found"` dead-ends are replaced by one shared `ensureMaterialized` helper.
- **Un-gate the Models section.** The detail page SHALL render the Models section for every available provider (configured or preset), sourced from the catalog. `Configured` is no longer a gate.
- **Status becomes connection-derived.** The "Not Configured" badge is replaced by a connection-derived pill: *Not connected* / *Connected* / *Cooldown* / *Awaiting credentials* (activated but no credential).
- **Delete reverts preset-backed providers.** Removing a preset-backed provider SHALL clear its topology entry and credentials, returning it to a clean available preset (it remains in the list), rather than making it disappear. Custom (non-preset) providers are removed entirely.
- **Out of scope (deferred):** relocating the static API key from plaintext-in-topology (`prov.APIKey`) into the credential store. This change preserves current storage behavior for parity; secret relocation is a follow-up change.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `management-dashboard`: The provider detail flow changes. The "must Configure first" gate is removed; connection and model-whitelisting actions SHALL work directly on available presets via lazy materialization; the Models section is no longer gated on `Configured`; the status badge becomes connection-derived; and deleting a preset-backed provider reverts it to a clean preset.

## Impact

- **Code:**
  - `internal/dashboard/handler.go` — `handleProviderDetailView` (merge preset+topology data, derive status), `handleProviderCredential`, `handleModelAdd`, `handleOAuthCallback` (lazy materialize via shared `ensureMaterialized` helper), `handleProviderDelete` (revert semantics for preset-backed providers). `handleProviderAdd` (Configure) is removed or demoted.
  - `internal/dashboard/view_provider_detail.templ` — remove the unconfigured Configure banner, un-gate the Models section (`if data.Configured`), replace the "Not Configured" badge with connection-derived status, add an "Awaiting credentials" state.
- **Tests:** `internal/dashboard/handler_test.go` — `TestProviderCRUDAndCredentials` and related cases currently assert the "must configure first" ordering; these flip semantics (save-key/whitelist-on-unconfigured-preset now succeeds). Expectations rewritten, not just extended.
- **Untouched:** routing/proxy layer (still reads topology — lazy writes propagate via the existing `TopologyWatcher`), credential store, preset definitions, CLI (`provider add` / `auth login`).
- **No breaking config changes:** topology file format and `${VAR}` references are preserved.
