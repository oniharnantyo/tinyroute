# Proposal: add-combo-wizard

## Why

Combos — named fallback chains where member 1 is tried first, member 2 on
failure, and so on — are fully supported by the routing engine (config schema,
combo resolution, ordered hop execution with health/quota/account fallback in
the proxy). But the only way to create one is `tinyroute combos add <name>
--members=...`, a flag-driven command that hard-errors the moment a value is
missing, violating the repo's interactive-first rule (`cli-interactivity.md`).
The dashboard has no combos surface at all. The users who benefit most from
priority fallback (subscription → cheap → free chains) have no guided way to
build one on either surface.

## What Changes

- **Rework `tinyroute combos add` into a step-by-step interactive wizard**
  (5 steps: name → members in priority order → mode → capabilities →
  review/confirm). Zero-arg runs the wizard in a TTY; typed args remain the
  scriptable shortcut (args-as-escape-hatch).
- **Ordered member selection**: members are picked one at a time ("Which model
  should be tried FIRST? SECOND? …"), so the pick sequence *is* the fallback
  order. Options come from live whitelisted models; already-chosen entries are
  disabled; a "Done — enough members" option ends the step (minimum 2).
- **Dashboard Combos section** (nav entry between Routes and History): list of
  combo cards with numbered member chips, create/edit via the same 5-step
  wizard rendered inside a dialog, delete behind an alert-dialog confirm,
  success feedback via toast.
- **Single source of truth**: both surfaces read/write `config.yaml` through
  the config service (same path as `provider add`); the watcher reloads live.
- **No engine changes**: ordered-mode execution, validation, and model
  discovery already work; this change is creation/management UX only.

No breaking changes — existing flags and config syntax are untouched.

## Capabilities

### New Capabilities

- `combo-management`: interactive-first combo creation and management on the
  CLI — the wizard flow, ordered member selection from live topology state,
  edge-state behavior (no providers, non-TTY, duplicate name), and the typed
  shortcut path.

### Modified Capabilities

- `management-dashboard`: the dashboard gains a combos management section —
  Combos navigation entry, combo list, dialog-hosted creation wizard, edit
  flow, and delete confirmation — composed from existing dashboard UI-kit
  components.

## Impact

- `internal/cli/combos.go` — `add` reworked into the wizard; `list`/`remove`
  unchanged.
- `internal/cli/interactive/` — reused as-is (Select, Input, MultiSelect,
  Confirm primitives).
- `internal/dashboard/` — new `view_combos.templ` (+ generated), handler
  additions for the wizard POST flow, nav entry in `view_layout.templ`.
- `internal/config/` — no schema changes; existing `Combo` fields and
  validation reused.
- Tests: CLI wizard unit tests (interactive-primitive fakes per existing
  patterns), dashboard handler tests.