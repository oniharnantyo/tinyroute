## Context

Combos are already routable by bare name (`Resolve` step 0) and by combo-as-prefix
with `$model` substitution (step 1, router.go:99-110). Client model pickers get
their options from `clients.DiscoverModelsForDialect`, which currently appends
bare combo names (uncommitted diff) — indistinguishable from models, grouped
under a misleading "defaults" bucket. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:**
- One canonical, self-documenting machine value (`combo:<name>`) across all
  model pickers, model listings, and written client configs
- Router accepts the key form with a minimal, composable rule
- Zero breakage: bare combo names and any existing provider named `combo`
  keep resolving exactly as before

**Non-Goals:**
- No filtering of picker entries (disabled / pure-`$model` combos stay listed —
  accepted sharp edges)
- No `$model` authoring support in the CLI/dashboard combo wizards
- No spec capture of the `$model` passthrough convention itself
- No `[1m]` suffix handling or whitelist checking on the combo-as-prefix path
  (pre-existing gaps, unchanged by this change)

## Decisions

### 1. Prefix-strip re-entry, not a parallel parse

`Resolve` checks `strings.HasPrefix(model, "combo:")` first and recurses (or
loops) with the remainder. No new parsing: a suffix-less remainder hits step 0
(bare combo lookup); an `x:y` remainder hits step 1 (combo-as-prefix with
`$model` substitution). `combo:cheap:gpt-4o` composes with passthrough for free.

*Alternative rejected:* a dedicated branch that interprets the key form
directly — duplicates the dispatch ladder and drifts from it.

### 2. Fallthrough beats forbidding a provider named `combo`

After the strip, if the remainder names no declared combo, resolution proceeds
down the provider path — so a provider literally named `combo` with whitelisted
models still serves `combo:<model>`. Combo lookup takes precedence.

*Alternative rejected:* rejecting a provider named `combo` at topology
validation — stricter, but breaks any existing config for marginal benefit.
We do reject a *combo* named `combo` (see next).

### 3. `ValidateComboName` rejects the name `combo`

Prevents `combo:combo` and nested-prefix ambiguity at authoring time. The
wizard, CLI, and dashboard all route through this validator, so one guard
covers every authoring surface.

### 4. Literal entries — no filters in discovery

`DiscoverModelsForDialect` emits `combo:<name>` for every declared combo.
Disabled combos fail at request time with the existing clear error
(`combo %q is disabled`); pure-`$model` combos called without a model suffix
send the literal `$model` upstream — both pre-existing behaviors, unchanged.
Documented here as accepted sharp edges.

### 5. Listings show one canonical form

`Router.Models(surface)` and `GET /{surface}/v1/models` list combos as
`combo:<name>` only. Bare names remain *resolvable* (back-compat) but are no
longer *listed* — pickers and configs converge on the key form. This supersedes
the uncommitted bare-name append in `installer.go`.

### 6. Dashboard grouping comes free

`groupModelsByProvider` splits on the first `:`; `combo:<name>` yields the
group key `combo`, rendered with a "Combos" header. The non-`:` fallback bucket
stays as-is — with key-form entries, combos no longer land in it, and the
duplicate-interleaved-"defaults" artifact disappears.

## Risks / Trade-offs

- [Two spellings coexist indefinitely] → bare name stays resolvable forever;
  mitigation is convergence: every tinyroute UI writes only the key form.
- [User picks a disabled combo at install time] → every request errors;
  mitigation: the runtime error names the combo and says it is disabled.
- [User picks a pure-`$model` combo without a model suffix] → literal `$model`
  upstream; pre-existing for bare names; revisit if wizard-side `$model`
  authoring lands (then this edge becomes common enough to guard).
- [Provider named `combo` shadowed by a same-named combo] → combo wins by
  precedence; the provider remains reachable via its other models.

## Migration Plan

Additive; no config migration. Client configs holding bare combo names keep
working (resolver back-compat). Rollback = revert; no persisted state changes.
