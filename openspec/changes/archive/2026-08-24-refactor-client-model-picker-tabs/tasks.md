## 1. Data shaping (tests first)

- [x] 1.1 Write table-driven tests for the reworked grouping: `provider:model` ids → models-only provider groups; `combo:<name>` ids → separate `[]ModelOption` with prefix-stripped labels; unprefixed ids → "defaults" group; empty input → empty outputs
- [x] 1.2 Rework `groupModelsByProvider` in `internal/dashboard/view_client_detail.go` to stop special-casing `combo:` and return models-only groups; add a combo-splitting helper (`Value: "combo:<name>"`, `Label: <name>`)
- [x] 1.3 Add `ComboOptions []ModelOption` to `ClientDetailPageData`; populate it and the models-only groups in `internal/dashboard/handler.go` client-detail data assembly (same `DiscoverModelsForDialect` output — no installer change)
- [x] 1.4 Run `go test ./internal/dashboard/` — grouping tests green

## 2. Dialog rework (templ)

- [x] 2.1 Extract the slot picker into a `slotPicker` templ component in `internal/dashboard` (dialog + tabs + inputs), keeping the trigger button, hidden slot input, and 1M checkbox wiring in the slot loop
- [x] 2.2 Build the Models pane: provider-grouped rows with per-group search input (input component inside inputgroup) bound to filter group `picker-<slotID>-models`; group wrapper divs carry `data-filter-item` with `groupSearchStrings` output as `data-filter-text` so headers hide on no-match
- [x] 2.3 Build the Combos pane: flat alphabetical name-only rows (same styled row component), own search input bound to `picker-<slotID>-combos`
- [x] 2.4 Render the `(None / Default)` clear row (optional slots) in both panes with the pane-local filter group
- [x] 2.5 Wrap panes in tabs (tabs component: `Tabs`/`List`/`Trigger`/`Content`, values `models`/`combos`) with `DefaultValue: "combos"` when the slot's current base value starts with `combo:`, else `"models"`; omit the tab bar and render the Models pane flat when `ComboOptions` is empty
- [x] 2.6 Run `templ generate` and `gofmt -w .`; confirm `go build ./...`

## 3. Verification

- [x] 3.1 Render tests: dialog with combos shows both tabs triggers and panes with disjoint filter groups; with no combos shows no tab bar; `DefaultValue` is `combos` for a `combo:` slot value and `models` otherwise; clear row present in both panes for optional slots; check indicator renders on the selected combo row
- [x] 3.2 Full suite: `go test ./...` green; update any tests pinned to the old single-list markup
- [x] 3.3 Manual pass at `/dashboard/clients/<client>`: tab switching, per-pane search (query isolation + group-header hiding), selecting a model and a combo end-to-end, clear row, dialog dismissal, no-combos client
