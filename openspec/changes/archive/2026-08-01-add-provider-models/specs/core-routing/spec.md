## ADDED Requirements

### Requirement: Direct Prefix Routing
The routing system SHALL intercept inbound model requests that follow the `provider:model` syntax and attempt a direct O(1) resolution against the requested provider's whitelist before falling back to manual route rules.

#### Scenario: Successful Direct Prefix Resolution
- **WHEN** an inbound request asks for model `openai:gpt-4o`
- **THEN** the router identifies `openai` as the provider, verifies that `gpt-4o` is in the `openai` whitelist, and directly returns the resolved route hop without iterating through wildcard manual routes.

### Requirement: Reject Unprefixed Requests
The routing system SHALL reject inbound model requests that do not specify a provider prefix, unless a legacy manual route explicitly matches the exact unprefixed string.

#### Scenario: Unprefixed Request Without Manual Route
- **WHEN** an inbound request asks for model `gpt-4o` and no manual route is configured for it
- **THEN** the router rejects the request with an error indicating that a provider prefix (e.g. `provider:gpt-4o`) is required.
