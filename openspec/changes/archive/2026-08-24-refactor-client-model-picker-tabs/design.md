## Context

See `proposal.md` — Why. Today `DiscoverModelsForDialect`
(`internal/clients/installer.go:103`) returns a flat sorted list of
`provider:model` ids with `combo:<name>` ids mixed in; `groupModelsByProvider`
(`internal/dashboard/view_client_detail.go:41`) folds the combo ids into a
pseudo-group named "Combos" and the dialog renders one scroll list with one
search input. Client-side filtering is `assets/filter.js`: an input carrying
`data-filter-input="<group>"` toggles `hidden` on every element carrying
`data-filter-item="<group>"`, matched globally across the document by
substring against `data-filter-text`. Tab behavior comes from the `tabs`
component (`components/tabs/`), whose script ships in the hashed bundle that
concatenates every `components/*/*.js` — nothing new to wire.

## Goals / Non-Goals

**Goals:**

- Tabbed Models/Combos picker with per-pane search, as specified in
  `specs/management-dashboard/spec.md`.
- Zero JavaScript changes — `filter.js` and `tabs.js` as-is.
- No change to the installer/read path (`DiscoverModelsForDialect` untouched).

**Non-Goals:**

- Member previews or any combo metadata in the picker (name-only rows).
- Changes to combo routing, `[1m]` suffix semantics, or slot persistence —
  `selectSlotModel` already handles arbitrary `data-model-val` values.
- Multi-select pickers or new slots.

## Decisions

### D1: Split models/combos at grouping time, not at the source

`groupModelsByProvider` is reworked to return models-only provider groups,
and combo ids become a separate `[]ModelOption` (`combo:<name>` →
`Value: "combo:<name>"`, `Label: <name>`), both stored on
`ClientDetailPageData`. Alternatives: splitting inside
`DiscoverModelsForDialect` (rejected — other callers may want the merged
list, and the handler's empty-fallback list at `handler.go:2088` stays
trivial), or adding a config-level helper (rejected —
`config.GetModelCandidates` already exists but returns no combos; a second
variant would duplicate the read).

### D2: Per-pane filter groups — `picker-<slotID>-models` / `picker-<slotID>-combos`

Each pane renders its own search input with its own group id; rows (and the
pane-local `(None / Default)` row) carry the matching `data-filter-item`.
`filter.js` scopes by group attribute, not DOM containment, so the two inputs
are independent by construction and each pane remembers its own query. A
shared single input driving one group across both panes was considered and
works equally well with zero JS, but per-pane bars were chosen (see proposal):
query isolation per category is the desired UX.

### D3: Filter-aware group wrappers via `groupSearchStrings`

The existing dormant helper `groupSearchStrings`
(`view_client_detail.go:117`) already builds `"provider", label, value, …`
for a group. Each provider group *wrapper* div gains
`data-filter-item="<models-group>"` with that string as its
`data-filter-text`, fixing the orphaned-header quirk: rows filter
independently by their own `data-filter-text` (which includes the provider
name), so a provider-name query keeps the wrapper plus all its rows, a
model-name query keeps the wrapper plus the matching row(s), and a
no-match query hides both. No parent-visibility logic needed in JS.

### D4: Tabs wiring and smart default pane

Each slot dialog renders one `tabs.Tabs` with `DefaultValue` computed
server-side: `"combos"` when the slot's current base value (suffix-stripped)
starts with `combo:`, else `"models"`. No explicit `Tabs.ID` — the
`utils.RandomID()` fallback (tabs.templ:63) keeps per-slot dialogs from
colliding. Panes: `tabs.Content{Value: "models" | "combos"}`. Tab-bar
omission when the combo list is empty is a templ conditional around the
`tabs.List` + two `tabs.Trigger`; the pane content still renders inside a
single non-tabbed container in that case (the Models pane markup is reused).

### D5: `(None / Default)` row duplicated per pane

The clear row renders once per pane (optional slots only). It carries no DOM
`id` — only `data-*` attributes consumed by the document-level click
delegation — so duplication is safe. The row keeps
`data-filter-text="none default"` in both panes' filter groups.

### D6: Extract the picker dialog body into a templ component

`view_client_detail.templ` is ~660 lines and the slot loop body grows with
tabs. The picker dialog (trigger button, dialog content, panes, search
inputs) moves into a dedicated `slotPicker` templ component in the same
package, keeping the slot loop readable. Composition stays within
shadcn-templ components (dialog, tabs, inputgroup, input, icon); the option
rows remain styled buttons as today — a raw element retained because no list
component fits single-pick rows (unchanged from current design).

## Risks / Trade-offs

- [Tab pane `hidden` and filter `hidden` fight each other] → They don't:
  tabs toggle `hidden` on the `tabs.Content` wrapper, filter.js toggles it
  on option rows — disjoint elements. Verified against `filter.js:20-27`
  and `tabs.templ:164-174`.
- [Selected check lands on an inactive pane] → Covered by the smart default
  pane (D4) and pinned by a spec scenario; add a render test for the
  `DefaultValue` computation.
- [Duplicate clear rows confuse the click delegation] → `closest()` returns
  the clicked instance; both share identical `data-slot-id`/`data-dialog-id`.
  Confirm with an existing-pattern test.
- [templ regeneration drift] → `templ generate` must run after editing
  `.templ`; `_templ.go` files are checked in.
- [Search a11y/keyboard inside a modal `<dialog>`] → Tab triggers remain
  focusable buttons inside the dialog's focus scope; no new JS. If arrow-key
  navigation in `tabs.js` proves awkward inside a modal, that is a pre-existing
  component behavior, not this change's to fix.

## Migration Plan

Pure UI refactor — no persisted state, API, or config changes. Deploy is a
normal build; rollback is a revert. Browser caches refresh via the content
hash in the bundle URL (no JS changes here, so the hash likely stays put).
