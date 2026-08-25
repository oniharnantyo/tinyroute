## ADDED Requirements

### Requirement: Combos section in dashboard navigation

The dashboard SHALL provide a Combos section reachable from the sidebar
navigation, positioned between Routes and History, using the Lucide layers
icon. The section SHALL list all configured combos.

#### Scenario: Combos nav entry is reachable

- **WHEN** an authenticated user views the dashboard
- **THEN** the sidebar SHALL contain a Combos entry between Routes and
  History, with the layers icon, linking to the combos list

### Requirement: Combo list with ordered members

The combos page SHALL render one card per combo (via the card component)
showing the combo name, a mode badge (via the badge component), and members as
position-numbered chips conveying member order. Each card SHALL offer edit
and delete actions. When no combos exist, the page SHALL show an empty state
(via the empty component with the layers icon) explaining what a combo is,
with a single call-to-action to create one.

#### Scenario: List renders numbered member order

- **WHEN** the combos page is viewed and a combo has members
  `["anthropic:claude-sonnet-4.5", "glm:glm-4.7", "openai:gpt-5.2"]`
- **THEN** the card SHALL display the members numbered 1, 2, 3 in that order

#### Scenario: Empty state with no combos

- **WHEN** no combos are configured
- **THEN** the page SHALL render an empty-state component describing combos
  with a create call-to-action, not a blank page

### Requirement: Combo creation wizard hosted in a dialog

Creating a combo SHALL happen inside a dialog (via the dialog component)
opened from the combos page's "New Combo" button, without navigating away
from the list. The wizard SHALL present the same five steps as the CLI wizard
(name, members in priority order, mode, optional capabilities, review) one
step at a time, with back/continue navigation, and SHALL NOT write
configuration until the final create action. Editing an existing combo SHALL
reuse the same dialog flow pre-filled with current values.

#### Scenario: Wizard opens in a dialog over the list

- **WHEN** the user activates "New Combo"
- **THEN** a dialog SHALL open over the combos list showing the name step
- **AND** the list SHALL remain visible behind it

#### Scenario: Step navigation within the dialog

- **WHEN** the user completes a step and continues, or goes back
- **THEN** the dialog SHALL advance or return one step, preserving previously
  entered values

#### Scenario: Nothing written until final create

- **WHEN** the user cancels or dismisses the dialog at any step
- **THEN** no configuration change SHALL occur

#### Scenario: Edit reuses the wizard pre-filled

- **WHEN** the user activates edit on a combo card
- **THEN** the same dialog wizard SHALL open pre-filled with that combo's
  current name, members in their configured order, mode, and capabilities

#### Scenario: Pinned members round-trip through edit

- **WHEN** the user edits a combo whose members include `glm@work:glm-4.7`
- **THEN** the wizard SHALL open with that member listed verbatim (pin
  intact)
- **AND** saving without changes SHALL preserve the member string exactly

### Requirement: Member selection conveys priority order

The members step SHALL add members one at a time through two dropdowns —
**Model** and **Connection** — appending each addition to a numbered list
whose position conveys member order. Each row SHALL offer move-up,
move-down, and remove controls (icon buttons via the button component).
At least one member SHALL be required to continue.

The Model dropdown SHALL list each whitelisted model exactly once per
provider (native `<select>` with `<optgroup>` provider grouping), in
`provider:model` form — the model list SHALL NOT be duplicated per account.
The Connection dropdown SHALL render visible but disabled with a
"Select a model first" placeholder until a model is chosen. When a model is
selected, the Connection dropdown SHALL be enabled and scoped to that
model's provider only: it SHALL offer **Any connection** (no account pin;
the provider's selection strategy applies) plus each account that provider
declares when it declares two or more (accounts in sorted order), defaulting
to the first account. When the selected model's provider declares fewer than
two accounts, the dropdown SHALL remain disabled on Any connection (the pin
would be semantically inert). The per-provider account list SHALL ship with
the page as a data island and the scoping SHALL be applied client-side on
model change — a presentational option-list sync only; all wizard state and
validation remain server-driven. Activating Add SHALL compose the member
server-side — selected model with Any connection yields `provider:model`;
selected model with account `work` yields `provider@work:model` — and SHALL
reject the addition with an inline error when the posted account belongs to
a provider other than the selected model's provider (a guard for crafted
posts; the scoped dropdown cannot produce this pairing) or when the composed
member is already in the list. After a successful add, both dropdowns SHALL
reset to their defaults.

#### Scenario: Added member appends as next priority

- **WHEN** two members are listed and a third is added
- **THEN** it SHALL appear as position 3 and the member order SHALL reflect
  the list top-to-bottom

#### Scenario: Reordering changes priority

- **WHEN** the user moves a member up one position
- **THEN** its position SHALL swap with the member above it

#### Scenario: A single member is enough to continue

- **WHEN** the members list holds one member and the user continues
- **THEN** the wizard SHALL advance to the mode step
- **AND** continuing with an empty list SHALL re-render the step with an
  inline error

#### Scenario: Model dropdown lists each model once per provider

- **WHEN** the members step renders for provider `glm` with accounts `work`
  and `personal` whitelisting `glm-4.7`
- **THEN** the Model dropdown SHALL contain `glm-4.7` exactly once under the
  `glm` group
- **AND** no `provider@account` options SHALL appear in the Model dropdown

#### Scenario: Connection dropdown is disabled until a model is selected

- **WHEN** the members step renders with no model chosen
- **THEN** the Connection dropdown SHALL be visible but disabled with a
  "Select a model first" placeholder

#### Scenario: Connection dropdown scopes to the selected model's provider

- **WHEN** the user selects Model `glm:glm-4.7` where `glm` declares
  accounts `work` and `personal`
- **THEN** the Connection dropdown SHALL be enabled with a `glm` group
  offering `personal` and `work`, plus Any connection
- **AND** it SHALL default to `glm`'s first account (`personal`, sorted
  first) with Any connection available as an explicit choice
- **AND** it SHALL NOT offer accounts of any other provider

#### Scenario: Single-account provider keeps the dropdown on Any

- **WHEN** the user selects a model of a provider declaring fewer than two
  accounts
- **THEN** the Connection dropdown SHALL remain disabled on Any connection

#### Scenario: Model plus connection composes a pinned member

- **WHEN** the user selects Model `glm:glm-4.7` and Connection `work` and
  activates Add
- **THEN** the member `glm@work:glm-4.7` SHALL be appended verbatim
- **AND** both dropdowns SHALL reset to their defaults

#### Scenario: Any connection composes an unpinned member

- **WHEN** the user selects Model `glm:glm-4.7` with Connection left at Any
  connection and activates Add
- **THEN** the member `glm:glm-4.7` SHALL be appended

#### Scenario: Mismatched connection is rejected

- **WHEN** a POST composes Model `openai:gpt-5.2` with Connection `work`
  (an account of `glm`)
- **THEN** the step SHALL re-render with an inline error naming the mismatch
- **AND** no member SHALL be appended

#### Scenario: Duplicate composed member is rejected

- **WHEN** Add is activated for a composed member identical to one already
  in the list
- **THEN** the step SHALL re-render with an inline error
- **AND** the member SHALL NOT be duplicated

### Requirement: Combo deletion requires confirmation

Deleting a combo SHALL require confirmation via a confirmation dialog (the
dialog component with destructive action styling, consistent with key
revocation) naming the combo. Only the confirm action SHALL remove the combo
from configuration.

#### Scenario: Delete confirms before removing

- **WHEN** the user activates delete on a combo card and confirms the
  confirmation dialog
- **THEN** the combo SHALL be removed and the list SHALL re-render without it

#### Scenario: Cancelled delete keeps the combo

- **WHEN** the user dismisses the delete confirmation
- **THEN** the combo SHALL remain configured and listed

### Requirement: Combo mutations notify via toast

Successful combo creation, edit, and deletion SHALL surface feedback through a
success toast (via the toast component) naming the combo and the action. The
toast SHALL contain no secrets — only the combo name, action, and member
count.

#### Scenario: Create success toast

- **WHEN** a combo is created through the wizard
- **THEN** a success toast SHALL appear with copy naming the created combo
- **AND** the toast SHALL NOT contain credentials or secrets

#### Scenario: Edit and delete toasts

- **WHEN** a combo is edited or deleted
- **THEN** a success toast SHALL appear naming the combo and the action
  performed

### Requirement: Connection removal keeps account-pinned combo members consistent

Removing a connection from the provider detail page SHALL downgrade every
combo member pinned to that account (`provider@account:model`) to its
unpinned form (`provider:model`), preserving the provider and model. When the
unpinned form is already a member of the same combo, the downgraded entry
SHALL be dropped instead of duplicated. A connection removal SHALL NOT
remove any combo — downgrade preserves the provider and model of every
member, so each combo keeps at least one member. The confirmation dialog SHALL
warn when combos will be affected, and the completion toast SHALL name every
combo that was modified. A combo SHALL never survive a connection removal
referencing an account the provider no longer declares.

#### Scenario: Pinned member downgrades on disconnect

- **WHEN** the user disconnects account `work` on provider `glm` and a combo
  holds member `glm@work:glm-4.7`
- **THEN** the member SHALL become `glm:glm-4.7` in the same write
- **AND** the toast SHALL name the combo that was modified

#### Scenario: Downgrade to an existing member drops the pin instead

- **WHEN** a combo holds both `glm:glm-4.7` and `glm@work:glm-4.7` and
  account `work` is disconnected
- **THEN** the pinned entry SHALL be dropped, leaving the member list
  unchanged in meaning and free of duplicates

#### Scenario: All-pinned combo survives downgrade with one member

- **WHEN** a combo's members all pin the disconnected account and downgrade
  would deduplicate them into one
- **THEN** the combo SHALL survive with that single unpinned member
- **AND** the toast SHALL name the combo that was modified

## MODIFIED Requirements

### Requirement: Connections can be reconnected and renamed from the dashboard

Each masked connection row on the provider detail page SHALL offer **Reconnect**
and **Rename** actions (via the row's `dropdown` menu). Reconnect SHALL start the
provider's OAuth flow with the row's account name as the explicit label, so
completed flows rotate that account's tokens in place. Rename SHALL re-key the
stored credential (`provider/<old>` → `provider/<new>`) and rewrite the matching
`Accounts[].Name` atomically in one gesture, after a `dialog` confirmation. A
rename to an existing name SHALL be rejected with a destructive `toast`. Success
SHALL show a success `toast` naming the account; no toast copy SHALL contain token
material. Rename SHALL additionally rewrite every combo member pinned to the old
account name (`provider@old:model` → `provider@new:model`) in the same write, so
no combo is left pinning an account the provider no longer declares.

#### Scenario: reconnect rotates a specific account in place

- **WHEN** the user picks Reconnect on connection `jane@example.com` and completes the flow
- **THEN** only that account's tokens SHALL be replaced
- **AND** a success `toast` SHALL name the account

#### Scenario: rename re-keys credential and topology together

- **WHEN** the user renames account `account-2` to `team-pool` and confirms the `dialog`
- **THEN** the credential store SHALL contain `provider/team-pool` and no `provider/account-2`
- **AND** the `Accounts[]` entry SHALL be renamed to `team-pool`
- **AND** a success `toast` SHALL name the new account

#### Scenario: rename to an existing name is rejected

- **WHEN** the user renames an account to a name already present on the provider
- **THEN** a destructive `toast` SHALL report the collision
- **AND** neither the credential store nor the topology SHALL change

#### Scenario: rename rewrites pinned combo members in the same write

- **WHEN** the user renames account `work` to `team` and a combo holds member
  `glm@work:glm-4.7`
- **THEN** that member SHALL become `glm@team:glm-4.7` in the same write as the
  `Accounts[]` rename
- **AND** a success `toast` SHALL name the account
