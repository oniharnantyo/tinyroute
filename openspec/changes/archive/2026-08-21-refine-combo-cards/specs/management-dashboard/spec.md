# management-dashboard Specification (delta)

## MODIFIED Requirements

### Requirement: Combo list with ordered members

The combos page SHALL render one card per combo (via the card component)
showing the combo name at the base text size (one step larger than body
text), a mode badge (via the badge component), and members as
position-numbered chips conveying member order. Member chips SHALL display
the `provider:model` form — any `@account` pin SHALL be stripped from the
card display while stored member strings remain verbatim (apparent
duplicates from same-model-different-account members SHALL render as-is).
Each card SHALL show at most 3 member chips; when a combo has more, an
overflow marker `+N more…` (N = total − 3) SHALL follow the list, carrying
the hidden members in its hover title. Disabled combos SHALL render
visually muted so their state is legible beyond the toggle position. Each
card SHALL offer enable/disable (see the toggle requirement), edit, and
delete actions. When no combos exist, the page SHALL show an empty state
(via the empty component with the layers icon) explaining what a combo is,
with a single call-to-action to create one.

#### Scenario: List renders numbered member order

- **WHEN** the combos page is viewed and a combo has members
  `["anthropic:claude-sonnet-4.5", "glm:glm-4.7", "openai:gpt-5.2"]`
- **THEN** the card SHALL display the members numbered 1, 2, 3 in that order

#### Scenario: Card title renders larger than body text

- **WHEN** a combo card is rendered
- **THEN** the combo name SHALL use the base text size, larger than the
  card's member and metadata text

#### Scenario: Member list caps at three with an overflow marker

- **WHEN** a combo has 5 members
- **THEN** the card SHALL render 3 member chips followed by `+2 more…`
- **AND** the overflow marker's hover title SHALL list the 2 hidden members

#### Scenario: Account pins are stripped from card display

- **WHEN** a combo member is stored as `glm@work:glm-4.7`
- **THEN** the card SHALL display it as `glm:glm-4.7`
- **AND** editing the combo SHALL still open with `glm@work:glm-4.7`
  verbatim

#### Scenario: Disabled combos render muted

- **WHEN** a combo has `disabled: true`
- **THEN** its card SHALL render with reduced emphasis relative to enabled
  cards

#### Scenario: Empty state with no combos

- **WHEN** no combos are configured
- **THEN** the page SHALL render an empty-state component describing combos
  with a create call-to-action, not a blank page

## ADDED Requirements

### Requirement: Combo cards can be toggled enabled or disabled

Each combo card SHALL carry an enable/disable switch (via the switch
component, `role=switch`) in the card footer, left of the edit action,
reflecting the combo's current state. Activating the switch SHALL submit a
form POST to `/dashboard/combos/toggle` carrying the combo name — no
client-side fetch or JavaScript beyond the shared component bundle. The
handler SHALL flip the combo's disabled flag, persist configuration, and
redirect back to the combos page with feedback. Because the gateway
watches the configuration file, the change SHALL take effect on the live
gateway without a restart.

#### Scenario: Toggle flips state and persists

- **WHEN** the user activates the switch on enabled combo `coding-priority`
- **THEN** the dashboard SHALL persist `disabled: true` for that combo
  and re-render the card with the switch off and muted styling

#### Scenario: Toggle takes effect without restart

- **WHEN** a combo is disabled from the dashboard while the gateway is
  running
- **THEN** the next request for that combo name SHALL fail with the
  explicit disabled error, with no gateway restart

#### Scenario: Toggle is a plain form POST

- **WHEN** the switch is rendered
- **THEN** it SHALL sit inside a form POSTing to `/dashboard/combos/toggle`
  with the combo name as a hidden field, covered by the dashboard's
  existing cross-origin protections
