## MODIFIED Requirements

### Requirement: Model picker options render as uniform rows in a modal dialog

Each model slot picker in the client detail editor SHALL open a modal dialog when activated.
Within the dialog, every option row — the `(None / Default)` clear option (for optional slots)
and each routable model — SHALL render as the same styled row component, and the currently
selected option SHALL be visibly marked. Selecting an option SHALL close the dialog and update
the slot's value; the styling of option rows SHALL NOT depend on client-side evaluation of an
expression that embeds static class names.

Options SHALL be grouped by identifier prefix: `provider:model` entries under a header naming
the provider, and `combo:<name>` entries under a single COMBOS header. Combos SHALL NOT render
under a "defaults" header, and a group header SHALL appear once per group rather than repeating
for interleaved entries. Selecting a combo row SHALL write the `combo:<name>` key form verbatim
into the slot.

#### Scenario: picker opens as a modal dialog

- **WHEN** the user activates a slot's picker button
- **THEN** a modal dialog opens, centered with a backdrop, listing the routable models grouped
  by provider with declared combos grouped under a single COMBOS header

#### Scenario: combo entries group under a single COMBOS header

- **WHEN** the picker dialog renders with declared combos present
- **THEN** every `combo:<name>` entry appears under one COMBOS header
- **AND** no combo entry appears under a "defaults" header
- **AND** selecting a combo row writes `combo:<name>` verbatim into the slot's value

#### Scenario: option rows render uniformly and styled

- **WHEN** the picker dialog renders
- **THEN** every option row — including `(None / Default)`, each model, and each combo — displays
  as the same bordered row component with consistent spacing
- **AND** no option renders as unstyled inline content

#### Scenario: the selected option is visibly marked

- **WHEN** the picker dialog opens for a slot that has a value
- **THEN** the row for the currently selected model carries a distinct selected-state accent
  and a check indicator

#### Scenario: selecting an option closes the dialog and updates the slot

- **WHEN** the user clicks an option row
- **THEN** the dialog closes and the slot's field displays the chosen model

#### Scenario: dialog is dismissible

- **WHEN** the picker dialog is open and the user presses Escape or clicks the backdrop
- **THEN** the dialog closes without changing the slot's value
