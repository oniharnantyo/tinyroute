# Proposal: add-combo-wizard

## Why

Combos — named multi-model panels (ordered sequences, load-balanced pools,
fused panels) — are fully supported by the routing engine (config schema,
combo resolution, ordered hop execution with health/quota/account fallback in
the proxy). But the only way to create one is `tinyroute combos add <name>
--members=...`, a flag-driven command that hard-errors the moment a value is
missing, violating the repo's interactive-first rule (`cli-interactivity.md`).
The dashboard has no combos surface at all. The users who benefit most from
priority chains (subscription → cheap → free) have no guided way to
build one on either surface.

Providers are now multi-connection (`add-multi-account-connections`): a
provider carries named accounts (`Provider.Accounts[]`), and the engine already
understands account-pinned combo members (`provider@account:model` — the
router parses the pin and the proxy restricts that hop to the named account's
credential). But no picker can produce one: member options derive from provider
whitelists only, so a user who wants a combo member drawn from a specific
connection (subscription account first, free account second) must hand-edit
YAML.

## What Changes

- **Rework `tinyroute combos add` into a step-by-step interactive wizard**
  (5 steps: name → members in priority order → mode → capabilities →
  review/confirm). Zero-arg runs the wizard in a TTY; typed args remain the
  scriptable shortcut (args-as-escape-hatch).
- **Ordered member selection**: members are picked one at a time ("Select the
  FIRST member: SECOND? …"), so the pick sequence *is* the member order.
  Options come from live whitelisted models; already-chosen entries are
  disabled; a "Done — enough members" option ends the step (at least one
  member required — combos serve ordered, pool, and fused modes, so there is
  no two-member floor).
- **Dashboard Combos section** (nav entry between Routes and History): list of
  combo cards with numbered member chips, create/edit via the same 5-step
  wizard rendered inside a dialog, delete behind an alert-dialog confirm,
  success feedback via toast.
- **Single source of truth**: both surfaces read/write `config.yaml` through
  the config service (same path as `provider add`); the watcher reloads live.
- **Account-aware member options**: when a provider declares multiple
  accounts, its models are offered both unpinned (`provider:model` — any
  account, the provider's selection strategy applies) and pinned to each
  declared account (`provider@account:model` — that connection only).
  Providers with zero or one account offer unpinned options only (a pin would
  be semantically inert). On the dashboard the members step presents this as
  two dropdowns — Model, and Connection (disabled until a model is chosen,
  then scoped to that model's provider and defaulting to its first account) —
  composed server-side on Add, so the model list is never duplicated per
  account.
- **Member validation closes the `model-combos` spec gap**: `ValidateTopology`
  rejects combo members that are malformed, reference an undeclared provider,
  or pin an unknown account — behavior the `model-combos` spec already
  requires but the implementation lacks (it validates route chains only).
  Both wizard write paths already gate through `ValidateTopology`.
- **Account lifecycle keeps pins consistent**: renaming an account rewrites
  pinned members to the new name; removing/disconnecting an account downgrades
  its pinned members to unpinned (provider + model preserved, combos never
  removed), so a connection change never leaves a combo referencing a
  nonexistent account.
- **No engine execution changes**: ordered-mode execution, hop resolution,
  account pinning, and model discovery already work; this change is
  creation/management UX plus the validation gap above.

No breaking changes — existing flags and config syntax are untouched.

## Capabilities

### New Capabilities

- `combo-management`: interactive-first combo creation and management on the
  CLI — the wizard flow, ordered member selection from live topology state
  (including account-pinned members for multi-connection providers),
  edge-state behavior (no providers, non-TTY, duplicate name), and the typed
  shortcut path.

### Modified Capabilities

- `management-dashboard`: the dashboard gains a combos management section —
  Combos navigation entry, combo list, dialog-hosted creation wizard, edit
  flow, and delete confirmation — composed from existing dashboard UI-kit
  components. Account rename and connection removal on the provider detail
  page additionally keep account-pinned combo members consistent.
- `provider-account-management`: `providers account remove` downgrades combo
  members pinned to the removed account instead of leaving dangling pins.

## Impact

- `internal/cli/combos.go` — `add` reworked into the wizard; `list`/`remove`
  unchanged.
- `internal/cli/interactive/` — reused as-is (Select, Input, MultiSelect,
  Confirm primitives).
- `internal/dashboard/` — new `view_combos.templ` (+ generated), handler
  additions for the wizard POST flow, nav entry in `view_layout.templ`;
  `handleProviderAccountRename` / `handleProviderCredentialDelete` updated to
  keep pinned members consistent.
- `internal/cli/account.go` — `providers account remove` downgrades pinned
  members of the removed account.
- `internal/config/` — no schema changes; `GetMemberCandidates` extended to
  emit account-pinned candidates; `ValidateTopology` gains the combo-member
  checks already required by the `model-combos` spec.
- Tests: CLI wizard unit tests (interactive-primitive fakes per existing
  patterns), dashboard handler tests, config validation tests.