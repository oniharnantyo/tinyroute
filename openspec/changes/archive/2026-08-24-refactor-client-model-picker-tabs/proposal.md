## Why

The client-detail model picker renders combos and models in one flat,
alphabetically sorted list. A combo entry (`combo:fast`) is indistinguishable
from a model row, is buried wherever `combo:` happens to sort among provider
names, and gives no hint that it routes differently. Combos are now a
first-class routing concept (the `add-combo-prefix-key` change landed); the
picker should expose them as a distinct, searchable category instead of
camouflaging them as models.

## What Changes

- The slot picker dialog body becomes a two-tab structure — **Models** and
  **Combos** — built on the existing `tabs` component (already in the hashed
  JS bundle; no new scripts).
- **Models tab**: provider-grouped model rows only, as today.
- **Combos tab**: flat, alphabetical, name-only combo rows.
- **Each tab pane has its own search input** with its own filter group
  (`picker-<slot>-models` / `picker-<slot>-combos`), so both tabs are
  independently searchable; each pane remembers its own query.
- The `(None / Default)` clear row (optional slots) renders in **both** panes.
- When no combos are configured, the tab bar is omitted entirely and the
  dialog renders the flat models list (today's behavior).
- The dialog opens on the **Combos** tab when the slot's current value is a
  `combo:` id, so the selected-state check is visible; otherwise on Models.
- Provider group wrappers become filter-aware during search, fixing the
  existing quirk where group headers stay visible after all their rows are
  filtered out.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `management-dashboard`: the model picker dialog requirement changes from a
  single flat "routable models grouped by provider" listing to a tabbed
  Models/Combos structure with per-tab search, a clear row in both panes,
  tab-bar suppression when no combos exist, and value-aware default tab.

## Impact

- `internal/dashboard/view_client_detail.go` — `groupModelsByProvider` stops
  special-casing `combo:`; page data gains a separate combo options list;
  the dormant `groupSearchStrings` helper is used to make group wrappers
  filter-aware.
- `internal/dashboard/view_client_detail.templ` — dialog body reworked into
  `tabs.Tabs` / `tabs.List` / `tabs.Trigger` / `tabs.Content` with per-pane
  search inputs.
- `internal/dashboard/handler.go` — client detail data assembly populates the
  new combo list.
- Tests covering the grouping helper, the dialog rendering, and handler data
  assembly.
- No JavaScript changes: `filter.js` and `tabs.js` work as-is.
- No changes to `DiscoverModelsForDialect` — combos still travel with the
  routable list and are separated at grouping time.
