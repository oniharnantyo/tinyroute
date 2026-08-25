## MODIFIED Requirements

### Requirement: Model picker options render as uniform rows in a modal dialog

Each model slot picker in the client detail editor SHALL open a modal dialog
(via the dialog component) when activated. The dialog body SHALL present two
tab panes built with the tabs component — **Models** and **Combos** — whenever
at least one combo is routable: the Models pane lists routable models grouped
by provider, and the Combos pane lists routable combos as a flat,
alphabetical, name-only list. When no combos are routable, the tab bar SHALL
be omitted and the dialog SHALL render the flat provider-grouped models list
directly. Each pane SHALL carry its own search input (via the input component)
that filters only that pane's option rows and group headers; a query in one
pane SHALL NOT affect the other. Within the dialog, every option row — the
`(None / Default)` clear option (for optional slots, rendered in both panes)
and each routable model or combo — SHALL render as the same styled row
component, and the currently selected option SHALL be visibly marked. The
dialog SHALL open with the Combos pane active when the slot's current value
is a combo id, and the Models pane active otherwise. Selecting an option
SHALL close the dialog and update the slot's value; the styling of option
rows SHALL NOT depend on client-side evaluation of an expression that embeds
static class names.

#### Scenario: picker opens as a modal dialog

- **WHEN** the user activates a slot's picker button and at least one combo is routable
- **THEN** a modal dialog opens, centered with a backdrop, with a Models pane listing
  the routable models grouped by provider and a Combos pane listing the routable
  combos as a flat alphabetical list

#### Scenario: tab bar is omitted when no combos are routable

- **WHEN** the picker dialog renders and no combos are routable
- **THEN** no Models/Combos tab bar renders, and the dialog shows the flat
  provider-grouped models list with its search input

#### Scenario: each pane filters with its own search input

- **WHEN** the user types a query into the Models pane's search input
- **THEN** only the Models pane's rows and group headers filter against the query,
  leaving the Combos pane's rows unaffected
- **AND** typing into the Combos pane's search input filters only the Combos pane

#### Scenario: group headers hide when no member rows match

- **WHEN** a search query in the Models pane matches no model of some provider group
- **THEN** that provider's group header is hidden along with its rows

#### Scenario: default tab follows the slot's current value

- **WHEN** the picker dialog opens for a slot whose current value is a combo id
- **THEN** the Combos pane is the active pane
- **WHEN** the picker dialog opens for any other slot value
- **THEN** the Models pane is the active pane

#### Scenario: option rows render uniformly and styled

- **WHEN** the picker dialog renders
- **THEN** every option row — including `(None / Default)`, each model, and each
  combo — displays as the same bordered row component with consistent spacing
- **AND** no option renders as unstyled inline content

#### Scenario: the selected option is visibly marked

- **WHEN** the picker dialog opens for a slot that has a value
- **THEN** the row for the currently selected model or combo carries a distinct
  selected-state accent and a check indicator

#### Scenario: selecting an option closes the dialog and updates the slot

- **WHEN** the user clicks an option row, in either pane
- **THEN** the dialog closes and the slot's field displays the chosen model or combo

#### Scenario: dialog is dismissible

- **WHEN** the picker dialog is open and the user presses Escape or clicks the backdrop
- **THEN** the dialog closes without changing the slot's value
