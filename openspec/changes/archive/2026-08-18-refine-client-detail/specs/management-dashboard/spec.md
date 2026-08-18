# management-dashboard Delta

## ADDED Requirements

### Requirement: Client detail header identifies the client by logo and status

The client detail view at `/dashboard/clients/{id}` SHALL render a header containing the
client's brand logo (looked up by client id from the embedded logo assets, with a monogram
fallback when no logo is embedded), the client's display name, and its status badge. The
header SHALL NOT render the raw client id/dialect metadata subtitle
(`ID: <id> • Dialect: <dialect>`).

#### Scenario: header shows the client's logo

- **WHEN** the user opens the detail view for a client with an embedded logo (e.g. `claude`)
- **THEN** the header renders that client's logo beside the client name and status badge

#### Scenario: header falls back to a monogram

- **WHEN** the user opens the detail view for a client with no embedded logo
- **THEN** the header renders a two-letter monogram derived from the client name

#### Scenario: header omits raw metadata

- **WHEN** the detail view renders
- **THEN** no `ID: <id> • Dialect: <dialect>` subtitle line appears in the header

### Requirement: Model picker options render as uniform rows in a modal dialog

Each model slot picker in the client detail editor SHALL open a modal dialog when activated.
Within the dialog, every option row — the `(None / Default)` clear option (for optional slots)
and each routable model — SHALL render as the same styled row component, and the currently
selected option SHALL be visibly marked. Selecting an option SHALL close the dialog and update
the slot's value; the styling of option rows SHALL NOT depend on client-side evaluation of an
expression that embeds static class names.

#### Scenario: picker opens as a modal dialog

- **WHEN** the user activates a slot's picker button
- **THEN** a modal dialog opens, centered with a backdrop, listing the routable models grouped
  by provider

#### Scenario: option rows render uniformly and styled

- **WHEN** the picker dialog renders
- **THEN** every option row — including `(None / Default)` and each model — displays as the
  same bordered row component with consistent spacing
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

### Requirement: Dashboard serves component behavior scripts

The dashboard SHALL serve the installed UI components' behavior scripts as static assets under
`/dashboard/assets/`, and the dashboard layout SHALL load the dialog component's script on
every page. The script SHALL be embedded in the binary (no runtime dependency on external
hosts other than the existing Alpine CDN tag).

#### Scenario: dialog script is served

- **WHEN** a logged-in session requests `GET /dashboard/assets/dialog.js`
- **THEN** the response is the dialog component's JavaScript with a 200 status

#### Scenario: layout loads the script

- **WHEN** any dashboard page renders
- **THEN** the page includes a `defer` script tag for `/dashboard/assets/dialog.js`