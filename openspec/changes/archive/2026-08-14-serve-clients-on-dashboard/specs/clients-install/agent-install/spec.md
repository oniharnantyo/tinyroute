## ADDED Requirements

### Requirement: Install orchestration is a reusable, medium-agnostic core

The client install logic SHALL be exposed as a medium-agnostic core that does not depend
on a TTY or any interactive-prompt primitive, so that both the CLI (`tinyroute clients
install`) and any non-interactive caller (the dashboard) drive the same flow. The core
SHALL provide:

- `Plan(req)` — resolves the adapter for the requested id, derives the base URL from the
  gateway listen address and the adapter dialect (honoring an explicit override),
  resolves the key strategy (mint a dialect-scoped key, or use a caller-supplied token),
  maps the requested model selections onto the adapter's declared slots, and returns a
  structured preview describing exactly what will be written and whether an existing file
  will be backed up — **without minting a key or writing any file**.
- `Apply(plan)` — mints the key (when the plan calls for it) and writes the configuration
  through the adapter's `Apply()`, returning the written file paths and the minted key
  (if any). Safe-write guarantees (backup, merge preserving unrelated user fields, atomic
  temp-file + rename at POSIX `0600`, idempotent re-install) SHALL hold.
- `MintKey(req)` — generates a `tr_live_` key scoped to the client's dialect
  (`Key.Allow = ["<dialect>:*"]`), persisted via the keystore, with a display name
  defaulting to `client-<id>` and overridable by the caller.

The CLI install command SHALL be re-implemented as a thin adapter that gathers inputs
through interactive prompts and delegates to this core; its observable behavior (client
selection, base-URL derivation, mint/reuse, model-slot mapping, preview, confirmation,
safe writes) SHALL be unchanged. A non-interactive caller SHALL satisfy the "Preview and
confirmation" requirement by obtaining a `Plan`, presenting it in its own medium, and
calling `Apply` only upon an explicit confirm — and SHALL satisfy the "Model selection"
requirement by supplying slot selections directly rather than via a prompt.

#### Scenario: Plan produces a preview without writing

- **WHEN** `Plan` is called for a known client
- **THEN** it returns a structured preview of the base URL, key strategy, slot mappings, and target paths
- **AND** no key is minted and no file is written

#### Scenario: Apply writes only after a confirmed plan

- **WHEN** `Apply` is called with a plan whose key strategy is mint
- **THEN** a dialect-scoped key is minted and the client config is written through the adapter
- **AND** the minted key is returned for one-time reveal

#### Scenario: Reuse writes the caller token as-is

- **WHEN** a plan's key strategy is reuse
- **THEN** `Apply` writes the caller-supplied token and mints no key

#### Scenario: the CLI delegates to the core with unchanged behavior

- **WHEN** `tinyroute clients install claude` runs interactively
- **THEN** the same base-URL derivation, mint/reuse choice, slot mapping, preview, and confirmation occur as before
- **AND** the underlying calls route through the shared core

#### Scenario: a non-TTY caller satisfies preview and confirmation

- **WHEN** a caller with no TTY requests an install
- **THEN** it obtains a `Plan`, presents the preview in its own medium, and applies only on explicit confirmation
- **AND** declining confirmation mints no key and writes nothing

#### Scenario: an unknown client is rejected at Plan time

- **WHEN** `Plan` is called for an id with no registered adapter
- **THEN** it returns an error and no plan is produced
