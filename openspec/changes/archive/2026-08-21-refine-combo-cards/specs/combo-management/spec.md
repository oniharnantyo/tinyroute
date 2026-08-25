# combo-management Specification (delta)

## ADDED Requirements

### Requirement: Combos can be disabled

A combo SHALL support a per-combo disabled flag, persisted as
`disabled: true` in configuration. A combo without the flag (or with it
false) SHALL be enabled — configurations written before this field existed
SHALL behave exactly as before. Disabling a combo SHALL NOT alter its
members, mode, or capabilities, and SHALL be reversible by clearing the
flag.

Requesting a disabled combo by name SHALL fail with an explicit error
naming the combo and stating it is disabled; resolution SHALL NOT silently
fall through to route-pattern matching. A parent combo enumerating a
disabled sub-combo member SHALL treat that member like any member whose
resolution fails — skip it and fall back to the next member (ordered
mode); a parent whose every member is disabled or unusable SHALL fail with
its normal all-members-failed error. Disabled combo names SHALL remain in
the model discovery list.

#### Scenario: Absent flag means enabled

- **WHEN** a configuration contains a combo without a `disabled` field
- **THEN** the combo SHALL resolve and route as enabled

#### Scenario: Direct request to a disabled combo errors explicitly

- **WHEN** a client requests model `coding-priority` and that combo has
  `disabled: true`
- **THEN** resolution SHALL fail with an error naming `coding-priority`
  and stating it is disabled
- **AND** the request SHALL NOT fall through to route-pattern matching

#### Scenario: Parent combo skips a disabled sub-combo member

- **WHEN** combo `team-pool` (ordered) has members
  `["coding-priority", "glm:glm-4.7"]` and `coding-priority` is disabled
- **THEN** resolving `team-pool` SHALL skip the disabled member and
  produce a chain starting at `glm:glm-4.7`

#### Scenario: Disabling preserves the combo definition

- **WHEN** a combo is disabled and later re-enabled
- **THEN** its members, order, mode, and capabilities SHALL be unchanged
