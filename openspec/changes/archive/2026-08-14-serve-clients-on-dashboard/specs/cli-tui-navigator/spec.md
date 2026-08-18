## MODIFIED Requirements

### Requirement: Commands are grouped into a navigable menu

The root `tinyroute` command SHALL register exactly 7 top-level subcommands:
`serve`, `init`, `keys`, `providers`, `combos`, `clients`, `history` — each
assigned to a named category so that `tinyroute --help` (and a bare `tinyroute`
invocation) renders a sectioned menu with 6 groups: Service (serve, init), Keys
(keys), Providers (providers), Combos (combos), Clients (clients), History
(history). The previously separate `provider` and `auth` commands SHALL be
merged into `providers` (with `auth` as a subcommand). The previously separate
`debug`, `status`, and `compact` commands SHALL be merged into `history` (as
`history log`, `history status`, `history compact`). The `validate` command
SHALL be removed (validation is implicit on `serve` startup and `combos add`).
A new `combos` command SHALL provide `add`, `list`, and `remove` subcommands
for managing `config.Topology.Combos`. The `clients` command (renamed from
`agent`) SHALL provide `install`, `status`, and `uninstall` subcommands for
configuring downstream coding clients. Running `tinyroute` with no subcommand
SHALL print this grouped menu (help).

#### Scenario: Bare invocation shows the grouped menu

- **WHEN** `tinyroute` is run with no subcommand
- **THEN** the output SHALL be the grouped command menu (help) with exactly 7 commands under 6 category headings
- **AND** no full-screen TUI SHALL be launched

#### Scenario: Help renders exactly 7 commands in 6 categories

- **WHEN** `tinyroute --help` is run
- **THEN** the command list SHALL show exactly these 7 commands: serve, init, keys, providers, combos, clients, history
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

#### Scenario: Clients command groups install, status, and uninstall

- **WHEN** `tinyroute clients --help` is run
- **THEN** the subcommands SHALL include `install`, `status`, and `uninstall`
- **AND** there SHALL be no top-level `agent` command
