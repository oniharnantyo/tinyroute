## MODIFIED Requirements

### Requirement: Model selection
Each adapter SHALL declare the model selections it supports — each shaped as a single pick or a
multi-list, and each optional or required. The install flow SHALL prompt for each declared
selection before writing config: a single selection via `Select`, a multi-list via `MultiSelect`,
with options sourced from the models tinyroute can route for the agent's dialect. Options SHALL
include one `combo:<name>` entry per declared combo, alongside the `provider:model` entries.
A selected combo entry SHALL be written verbatim in its `combo:<name>` key form. Selections SHALL
be skippable unless declared required. When no routed models exist, a required selection SHALL
fall back to free-text entry and an optional selection SHALL be skipped. Adapters that declare no
model selections SHALL skip model prompting entirely. A `--model` flag SHALL pre-fill any single
selection and skip its prompt; the flag SHALL accept the `combo:<name>` key form.

#### Scenario: Single-model agent uses routed models
- **WHEN** installing an agent that declares one required single-model slot, interactively
- **THEN** the user is offered the models routable on that agent's dialect and the selection is written to the agent's model field

#### Scenario: Multi-list agent selects several models
- **WHEN** installing an agent that declares a multi-list slot (e.g. copilot), interactively
- **THEN** the user is offered a multi-select of routable models and the chosen list is written to the agent's config

#### Scenario: Combo entries appear in key form
- **WHEN** at least one combo is declared and the install flow prompts for a model selection
- **THEN** the offered options include `combo:<name>` for each declared combo
- **AND** selecting one writes `combo:<name>` verbatim into the agent's model field

#### Scenario: Optional slot may be skipped
- **WHEN** the user skips an optional model slot
- **THEN** the agent's corresponding model field is left untouched by tinyroute

#### Scenario: Agent without model selections skips prompting
- **WHEN** installing an agent that declares no model slots (e.g. devin)
- **THEN** no model prompt is shown and only base URL + key (if any) are configured
