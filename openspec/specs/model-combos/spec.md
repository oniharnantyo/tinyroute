# model-combos Specification

## Purpose

Defines model combos: named logical-model entries that resolve to an
ordered/pooled/fused panel of provider-model members, with capability reorder.

## Requirements

### Requirement: A named combo expands to an ordered/pooled/fused panel

The system SHALL support `combos` in configuration: each combo has a `name`, an
ordered list of `members` (`provider/model` or `provider@account/model`), a
`mode` of `ordered`, `pool`, or `fused`, and an optional capability list. A combo
name SHALL be resolvable as a model on a matching surface, expanding to its
member panel.

#### Scenario: A combo name resolves to its member chain

- **WHEN** a client requests a model that matches a declared combo name
- **THEN** the router SHALL expand it into the combo's member hops in order

#### Scenario: Ordered mode behaves like today's attempt loop

- **WHEN** a combo has `mode: "ordered"`
- **THEN** members SHALL be attempted sequentially, stopping at the first success

#### Scenario: Pool mode picks the first concurrent success

- **WHEN** a combo has `mode: "pool"`
- **THEN** members SHALL be fanned out concurrently and the first successful response SHALL be returned

### Requirement: Fusion mode fans out with quorum and synthesis

The system SHALL support `mode: "fused"`: members SHALL be queried in parallel,
a quorum of member successes SHALL be required, and the results SHALL be
synthesized by a judge model before being returned. For streaming, fusion SHALL
be best-effort. Fusion SHALL be an opt-in per-combo behavior and SHALL NOT be run
for `ordered` or `pool` combos.

#### Scenario: Fused combo requires quorum before emitting

- **WHEN** a `fused` combo is requested and fewer members than the quorum succeed
- **THEN** the request SHALL be treated as a failure and SHALL NOT emit a synthesized response

#### Scenario: Non-fused combos do not pay fusion cost

- **WHEN** a client requests an `ordered` or `pool` combo
- **THEN** no judge-model synthesis and no quorum evaluation SHALL occur

### Requirement: Capability reorder tiers the panel

The system SHALL support capability-based reordering of combo members. A
capability tier SHALL be associated with each member, ordered by a fixed
hierarchy (`vision` > `pdf` > `audio` > `video`). When a request requires only a
subset of capabilities, members lacking the highest needed tier SHALL be
excluded from the panel before selection.

#### Scenario: Members lacking a needed capability are excluded

- **WHEN** a request requires only `vision` and a member does not support `vision`
- **THEN** that member SHALL be excluded from the active panel
- **AND** the remaining capable members SHALL be used

#### Scenario: Hard-cap tiering prescribes the active panel

- **WHEN** a request needs a specific capability tier
- **THEN** the panel SHALL contain only members whose tier meets or exceeds the required tier, applied in the fixed hierarchy order

### Requirement: Combos appear in model discovery

The system SHALL list combo names in model discovery for a surface, and a listed
combo name SHALL resolve successfully on that surface. Discovery and resolution
SHALL agree for combos exactly as for provider whitelist models.

#### Scenario: A combo name is discoverable and resolvable

- **WHEN** a combo is declared and `GET /{surface}/v1/models` is requested on a matching surface
- **THEN** the combo name SHALL appear in the listing
- **AND** sending the combo name as the model SHALL resolve without `404`

### Requirement: Combo validation

`ValidateTopology` SHALL reject a combo whose members reference an undeclared
provider or an unknown account, whose name collides with a provider-whitelist
model on the same surface, or whose `mode` is not one of `ordered`, `pool`, or
`fused`.

#### Scenario: Combo with an unknown provider is rejected

- **WHEN** a combo member references a provider that is not declared
- **THEN** validation SHALL return an error naming the provider and the combo

#### Scenario: Combo with an unknown mode is rejected

- **WHEN** a combo declares `mode` other than `ordered`/`pool`/`fused`
- **THEN** validation SHALL return an error naming the combo and the invalid mode
