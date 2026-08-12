# Tasks

## 1. Materialization seam (foundation)

- [x] 1.1 Add `ensureMaterialized(rawTopo, provName) (materialized bool, err error)` helper in `internal/dashboard/`: if `provName` is a known preset AND absent from `rawTopo.Providers`, write an entry built from preset defaults (reuse logic from `handleProviderAdd` dialect/baseURL/`${VAR}` mapping); no-op (return false) when already present or not a preset.
- [x] 1.2 Add unit tests for the helper: preset-not-in-topo materializes with `${VAR}` preserved; already-present is a no-op; non-preset unknown name is a no-op returning a "not available" signal; write is atomic at mode `0600`.

## 2. Detail-view handler: availability + derived status

- [x] 2.1 In `handleProviderDetailView`, treat a provider as available when `topo.Providers[name]` OR `preset.Get(name)` exists; redirect "not found" only when neither is present.
- [x] 2.2 Resolve effective dialect/baseURL by merging preset defaults with topology overrides; load the catalog + whitelist for every available provider (remove the `if configured` gate around catalog loading at `handler.go:519`).
- [x] 2.3 Derive a `Status` field on `ProviderDetailPageData` from connection state: `not_connected` (available preset, no topology entry), `awaiting_credentials` (in topology, no credential), `connected`, `cooldown`. Keep `Configured` only where the template still needs topology-membership, or remove if unused.
- [x] 2.3a Set `OAuthCapable` from the preset for available presets (already true) and confirm connection list still sources from the credential store for available presets.

## 3. Wire mutation handlers to lazy materialization

- [x] 3.1 `handleProviderCredential`: replace the `if !ok { "Provider not found" }` block (`handler.go:895`) with `ensureMaterialized`; on materialize-but-not-preset (truly unknown), keep the clear error. Then store `prov.APIKey` as today.
- [x] 3.2 `handleModelAdd`: replace the `if !ok` block (`handler.go:986`) with `ensureMaterialized`, then append the model. Reject only genuinely unknown names.
- [x] 3.3 `handleOAuthCallback`: call `ensureMaterialized` for the provider before/within storing the OAuth credential, so a just-connected preset is routable. Verify idempotency when the entry already exists.

## 4. Provider detail template

- [x] 4.1 Remove the "UNCONFIGURED ENCOURAGEMENT BANNER" block and its Configure button (`view_provider_detail.templ:124-147`).
- [x] 4.2 Remove the `if data.Configured` gate around the Models section (`view_provider_detail.templ:225`) so it renders for every available provider.
- [x] 4.3 Replace the header "Not Configured" badge (`view_provider_detail.templ:60-68`) with a connection-derived pill: Not connected / Awaiting credentials / Connected / Cooldown, driven by the new `Status` field.
- [x] 4.4 Regenerate templ output (`templ generate`) and confirm the page renders for an available preset with no topology entry.

## 5. Delete revert semantics

- [x] 5.1 `handleProviderDelete`: when `preset.Get(provName) != nil`, after removing the topology entry also clear any stored credentials for that provider; redirect to the providers list where the preset reappears as an unconnected card. For non-preset providers, behavior is unchanged (full removal).

## 6. Remove standalone Configure action

- [x] 6.1 Remove or demote `handleProviderAdd` / the `POST /dashboard/providers/add` route now that no UI surface triggers it; keep the route only if retained as an explicit activation escape hatch (decide per Open Question 1). If removed, delete its test references.

## 7. Tests

- [x] 7.1 Rewrite `TestProviderCRUDAndCredentials` and neighbors in `handler_test.go`: saving an API key on an unconfigured preset now succeeds and materializes the provider; whitelisting a model on an unconfigured preset succeeds.
- [x] 7.2 Add a test: completing/stubbing an OAuth callback for an unconfigured preset materializes it into the topology.
- [x] 7.3 Add a test: deleting a preset-backed provider clears its topology entry + credentials and it reappears as an available card; deleting a custom provider removes it.
- [x] 7.4 Add a test: detail view for an available preset renders the Models section and shows "Not connected" status (no Configure banner).
- [x] 7.5 Add a test: mutation on a genuinely unknown provider name is still rejected.

## 8. Verification

- [x] 8.1 `gofmt -w .` and `go build ./...`.
- [x] 8.2 `go test ./internal/dashboard/...` passes with updated/added cases.
- [x] 8.3 Manual: open an unconfigured preset detail page → paste an API key → confirm it saves and the provider becomes routable without ever pressing Configure; confirm Models section renders immediately.
- [x] 8.4 `openspec validate collapse-provider-configuration` passes; reconcile any spec drift before archive.

