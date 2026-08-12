# cli-tui-navigator Specification

## Purpose

Defines the slim 7-command base grouped into navigable categories (Service,
Keys, Providers, Combos, Agents, History), replacing the earlier full-screen
TUI navigator and the flat command list.

## ADDED Requirements

### Requirement: Slim 7-command base with grouped categories

The root `tinyroute` command SHALL register exactly 7 top-level subcommands:
`serve`, `init`, `keys`, `providers`, `combos`, `agent`, `history` — each
assigned to a named category so that `tinyroute --help` (and a bare `tinyroute`
invocation) renders a sectioned menu with 6 groups: Service (serve, init), Keys
(keys), Providers (providers), Combos (combos), Agents (agent), History
(history). The previously separate `provider` and `auth` commands SHALL be
merged into `providers` (with `auth` as a subcommand). The previously separate
`debug`, `status`, and `compact` commands SHALL be merged into `history` (as
`history log`, `history status`, `history compact`). The `validate` command
SHALL be removed (validation is implicit on `serve` startup and `combos add`).
A new `combos` command SHALL provide `add`, `list`, and `remove` subcommands
for managing `config.Topology.Combos`. Running `tinyroute` with no subcommand
SHALL print this grouped menu (help).

#### Scenario: Bare invocation shows the grouped menu

- **WHEN** `tinyroute` is run with no subcommand
- **THEN** the output SHALL be the grouped command menu (help) with exactly 7 commands under 6 category headings
- **AND** no full-screen TUI SHALL be launched

#### Scenario: Help renders exactly 7 commands in 6 categories

- **WHEN** `tinyroute --help` is run
- **THEN** the command list SHALL show exactly these 7 commands: serve, init, keys, providers, combos, agent, history
- **AND** each command SHALL appear under exactly one category

#### Scenario: Providers includes auth as a subcommand

- **WHEN** `tinyroute providers --help` is run
- **THEN** the subcommands SHALL include `add`, `auth`, and `model`
- **AND** there SHALL be no top-level `auth` command

#### Scenario: History includes status and compact as subcommands

- **WHEN** `tinyroute history --help` is run
- **THEN** the subcommands SHALL include `log`, `sessions`, `status`, and `compact`
- **AND** there SHALL be no top-level `debug`, `status`, or `compact` command

#### Scenario: Combos add writes to config

- **WHEN** `tinyroute combos add mycombo --members=openai:gpt-4,anthropic:claude-3 --mode=pool` is run
- **THEN** the combo SHALL be persisted to `config.json` under the `combos` array
- **AND** `tinyroute combos list` SHALL show the combo with its mode and members
- **AND** `tinyroute combos remove mycombo` SHALL remove it from config

## REMOVED Requirements

### Requirement: Full-screen navigator shell

**Reason**: The full-screen Bubble Tea TUI (`tinyroute tui`) was descoped in
favor of a grouped command menu on the existing command base. The TUI
introduced a parallel presentation layer with placeholder panes and duplicate
data-loading effort, whereas the command base already provides the functional
operations. The grouped menu gives users the same navigable structure without
the TUI's complexity or dependency weight.

**Migration**: Use the grouped command menu (`tinyroute --help`). The
`tinyroute tui` subcommand and `internal/cli/tui` package have been removed.
The Bubble Tea / bubbles / lipgloss dependencies have been dropped from
`go.mod`. The `tinyroute init` interactive wizard reverts to the existing
`interactive.RunInitWizard` (pterm-based) flow.

### Requirement: Function-key actions and modals

**Reason**: Descoped with the full-screen TUI — function-key shortcuts and
modal confirmations are TUI-specific interaction patterns. The command base
uses the existing interactive prompt/confirm primitives (`interactive.Input`,
`interactive.Confirm`, `interactive.Select`) and the `--no-interactive`/
`--force` flags instead.

**Migration**: Use the existing command flags and interactive prompts. For
scriptable non-interactive use, pass `--no-interactive` or `--force`.

### Requirement: Inline key and provider management

**Reason**: Descoped with the full-screen TUI — inline pane-based CRUD is
replaced by the existing `tinyroute keys`, `tinyroute provider`, and
`tinyroute auth` command groups, which already provide add/edit/delete through
the interactive-first CLI pattern.

**Migration**: Use `tinyroute keys create/list/revoke`, `tinyroute provider
add/list/model`, and `tinyroute auth login/set/list`.

### Requirement: Live status and logs

**Reason**: Descoped with the full-screen TUI — the Status and Live Logs panes
are replaced by the existing `tinyroute status` and `tinyroute debug log`
commands, which read from the same persisted sources (`state.json` and the
SQLite history database).

**Migration**: Use `tinyroute status` for cooldown/health and `tinyroute debug
log` for recent request history.

### Requirement: Full-screen setup app

**Reason**: Descoped with the full-screen TUI — `tinyroute init` reverts to the
existing pterm-based interactive wizard (`interactive.RunInitWizard`), which
already provides multi-step guided setup with back navigation and transactional
save. The full-screen setup app added a second wizard implementation with no
functional gain.

**Migration**: Use `tinyroute init` (interactive in a TTY, scaffold in
non-TTY).

### Requirement: Panes render live data and support read/write actions

**Reason**: Descoped with the full-screen TUI — pane-based data rendering is
replaced by the command base's existing table/list output, which renders real
data from the same services.

**Migration**: Use the existing commands (e.g. `tinyroute provider list`,
`tinyroute keys list`, `tinyroute status`, `tinyroute debug log`).

### Requirement: Status and live logs use persisted, cross-process sources

**Reason**: Descoped with the full-screen TUI — the existing `tinyroute status`
and `tinyroute debug log` commands already read from persisted cross-process
sources (`state.json` and the SQLite history database).

**Migration**: Use `tinyroute status` and `tinyroute debug log`.
