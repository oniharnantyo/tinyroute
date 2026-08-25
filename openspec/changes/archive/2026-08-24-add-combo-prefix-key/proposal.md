## Why

Combos are routable by bare name today, but in client model pickers a bare combo
name is indistinguishable from a provider model, renders under a misleading
"defaults" group (occasionally as several interleaved groups), and conflates the
human display label with the machine identifier written into client configs. A
`combo:<name>` key form gives every picker one unambiguous, self-documenting
machine value — the string in `~/.codex/config.toml` says what it is.

## What Changes

- **Router accepts the `combo:` prefix key.** `combo:<name>` resolves identically
  to the bare combo name; `combo:<name>:<model>` composes with the existing
  combo-as-prefix `$model` passthrough. Implementation is a single prefix-strip
  before the existing dispatch ladder — no new parsing rules.
- **Bare combo names keep working** (back-compat). A provider literally named
  `combo` also keeps working via fallthrough: if the remainder names no combo,
  resolution proceeds down the provider path.
- **Combo naming guard.** `ValidateComboName` rejects a combo named `combo`
  (prevents `combo:combo` and nested-prefix ambiguity).
- **Discovery emits key-form entries.** `DiscoverModelsForDialect` offers
  `combo:<name>` for every declared combo — literal, no filtering. Disabled
  combos and pure-`$model` combos picked bare remain known sharp edges (their
  runtime failure modes are pre-existing), documented in design.md.
- **Model listing agrees with the resolver.** `GET /{surface}/v1/models` lists
  `combo:<name>` identifiers so no offered id is rejected when sent back
  (per the existing listing/resolver agreement requirement).
- **Dashboard picker groups combos under a COMBOS header.** With `combo:` being
  a `:`-prefixed id, existing provider grouping buckets them for free; the
  header renders as a labeled COMBOS section instead of "defaults".

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `core-routing`: resolver identifier forms gain `combo:<name>` and
  `combo:<name>:<model>`; model discovery lists combo key-form identifiers.
- `clients-install`: model selection options include `combo:<name>` entries;
  `--model` accepts the key form.
- `management-dashboard`: client detail model pickers render combo entries
  grouped under a COMBOS header (replacing the "defaults" bucket for unprefixed
  ids).

## Impact

- `internal/route/router.go` — prefix-strip rule, fallthrough, `Models()`
- `internal/config/combos.go` — `ValidateComboName` guard
- `internal/clients/installer.go` — `DiscoverModelsForDialect` emits key form
- `internal/cli/clients.go` — install picker (no structural change; entries arrive keyed)
- `internal/dashboard/view_client_detail.go`, `handler.go` — grouping/labels
- Tests: `router_test.go`, `combos_test.go` (config + cli), `installer_models_test.go`,
  `handler_test.go`
