# core-routing Specification (delta)

## ADDED Requirements

### Requirement: Resolver handles account-pinned and combo hops

The router SHALL resolve `provider@account:model` notation, `provider@default:model`
(and bare `provider:model`), and named combo models into a `ResolvedRoute` whose
hops may carry an optional account and pool/combo intent. Resolution SHALL agree
with model discovery for account-pinned and combo identifiers.

#### Scenario: Account-pinned model resolves to that account

- **WHEN** a client requests `provider@account:model` and the account is declared
- **THEN** the resolved hop SHALL pin `provider/account` with `model`
- **AND** SHALL resolve without error

#### Scenario: Combo name resolves through the panel

- **WHEN** a client requests a model matching a declared combo name
- **THEN** the router SHALL return the expanded combo member hops with their mode and capability intent

#### Scenario: Unknown account or combo is a resolution error

- **WHEN** a client requests `provider@nope:model` or an undeclared combo name
- **THEN** resolution SHALL fail with an error naming the missing account or combo

### Requirement: Model discovery lists combos and account-pinned models

Model discovery SHALL include declared combo names and account-pinned identifiers
when they resolve successfully on the surface. Every returned identifier SHALL
resolve when sent to that surface's generation endpoint.

#### Scenario: Combos appear in discovery and resolve

- **WHEN** a combo is declared and model discovery runs for a matching surface
- **THEN** the combo name SHALL be listed
- **AND** sending it as the model SHALL resolve without `404`

### Requirement: Cross-dialect translation applies to combo members

Each combo member SHALL be treated like an ordinary hop for translation: a member
whose provider dialect differs from the surface SHALL require a registered
translator, exactly as a non-combo hop does.

#### Scenario: Combo member requiring translation resolves only when translatable

- **WHEN** a combo member's provider dialect differs from the surface and no translator is registered
- **THEN** the combo SHALL be rejected by validation/resolution with a clear error
