# Proposal: refine-combo-cards

## Why

The dashboard combos page works, but the cards leak wizard-level detail and
lack a runtime control. Three frictions:

- **Cards grow unbounded with member count.** A pool combo with six members
  renders six rows of `provider@account:model` strings — long enough to break
  the 3-column grid's rhythm, and the `@account` pin is connection plumbing
  that belongs in the wizard, not at a glance.
- **No way to park a combo.** Taking a combo out of rotation today means
  deleting it (losing the member list) or hand-editing YAML. Neither is
  reversible in one click.
- **The card title renders at `text-sm`** — smaller than the content it
  labels, so cards scan as body text rather than entries.

The gateway already hot-reloads `config.yaml` through the topology watcher
(`serve.go`), so an enable/disable flag persisted from the dashboard takes
effect on the live gateway with no restart — the feature is cheap
operationally.

## What Changes

- **Combo cards show at most 3 members**, each displayed as `provider:model`
  (the `@account` pin is stripped from display; duplicates render as-is).
  When a combo has more than 3 members, an overflow marker (`+N more…`)
  follows the list, with the hidden members listed in its hover `title`.
- **Combo cards carry an enable/disable switch** in the footer (left of
  Edit), rendered as a `role=switch` button inside a form POST to
  `/dashboard/combos/toggle` — zero JavaScript, consistent with the
  dashboard's pure form-POST architecture. Disabled cards render visually
  muted.
- **Combo schema gains a `disabled` flag** (`disabled: true` in YAML; absent
  means enabled — existing configs are unaffected).
- **Router semantics for disabled combos**: requesting a disabled combo by
  name returns an explicit error (`combo %q is disabled`) rather than
  silently falling through to route patterns; a parent combo treats a
  disabled sub-combo member like any failed member (skipped, with fallback
  to the next member in ordered mode).
- **Card title renders at `text-base`** instead of `text-sm`.
- New **switch component** vendored into `internal/dashboard/components/`
  from shadcn-templ, following the existing component layout.

## Non-Goals

- CLI surfacing of the enabled state (`tinyroute combos list` output) — can
  follow later if wanted.
- Removing disabled combos from the models-discovery list (`Models()`) —
  they remain listed; the request error is explicit.
- Changing the wizard's member display (full pinned strings stay verbatim
  in the wizard, edit round-trip, and review step).

## Impact

- `internal/config/topology.go` — `Combo.Disabled` field
- `internal/route/router.go` — disabled checks at both combo resolution
  entry points and during member enumeration
- `internal/dashboard/combos.go` — toggle POST handler
- `internal/dashboard/view_combos.templ` — card refinements
- `internal/dashboard/components/switch/` — new component
- Specs: `combo-management` (schema + routing), `management-dashboard`
  (card display + toggle)
