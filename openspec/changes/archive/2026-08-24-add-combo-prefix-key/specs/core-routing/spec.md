## MODIFIED Requirements

### Requirement: Resolver handles account-pinned and combo hops

The router SHALL resolve `provider@account:model` notation, `provider@default:model`
(and bare `provider:model`), and named combo models into a `ResolvedRoute` whose
hops may carry an optional account and pool/combo intent. Resolution SHALL agree
with model discovery for account-pinned and combo identifiers.

Combos SHALL be addressable by the `combo:` key form. `combo:<name>` SHALL
resolve identically to the bare combo name `<name>`, and `combo:<name>:<model>`
SHALL resolve through the same combo-as-prefix behavior as `<name>:<model>`.
When the identifier following the `combo:` prefix names no declared combo and a
provider named `combo` is declared, resolution SHALL proceed down the provider
path, preserving back-compat for both spellings.

A model identifier without a provider prefix SHALL resolve only when it names a
declared combo. Any other unprefixed identifier SHALL fail resolution with an
error naming the model and stating the two supported forms: a `provider:model`
prefix or a declared combo. A `combo:`-prefixed identifier that names neither a
declared combo nor (via fallthrough) a model of a provider named `combo` SHALL
fail resolution with an error naming the identifier.

#### Scenario: Account-pinned model resolves to that account

- **WHEN** a client requests `provider@account:model` and the account is declared
- **THEN** the resolved hop SHALL pin `provider/account` with `model`
- **AND** SHALL resolve without error

#### Scenario: Combo name resolves through the panel

- **WHEN** a client requests a model matching a declared combo name
- **THEN** the router SHALL return the expanded combo member hops with their mode and capability intent

#### Scenario: combo key form resolves the named combo

- **WHEN** a client requests `combo:<name>` and `<name>` is a declared, enabled combo
- **THEN** the router SHALL return the same expanded hops as the bare name `<name>`

#### Scenario: combo key form composes with model passthrough

- **WHEN** a client requests `combo:<name>:<model>` and `<name>` is a declared combo
  with a `$model` member
- **THEN** the router SHALL resolve through the combo panel with `<model>`
  substituted into the `$model` member, matching the `<name>:<model>` form

#### Scenario: combo prefix falls through to a provider named combo

- **WHEN** a provider named `combo` is declared with whitelisted model `<model>`
- **AND** no combo named `<model>` is declared
- **AND** a client requests `combo:<model>`
- **THEN** the router SHALL resolve to the `combo` provider's hop for `<model>`

#### Scenario: Unknown account or combo is a resolution error

- **WHEN** a client requests `provider@nope:model` or an undeclared combo name
- **THEN** resolution SHALL fail with an error naming the missing account or combo

#### Scenario: Unprefixed non-combo model is a resolution error

- **WHEN** a client requests a bare model name (no `provider:` prefix) that is not
  a declared combo
- **THEN** resolution SHALL fail with an error naming the model and stating that a
  `provider:model` prefix or a declared combo is required
- **AND** the error SHALL NOT reference route configuration

#### Scenario: combo prefix naming neither combo nor provider model errors

- **WHEN** a client requests `combo:<name>` where `<name>` is neither a declared
  combo nor a whitelisted model of a provider named `combo`
- **THEN** resolution SHALL fail with an error naming the identifier

### Requirement: Model discovery lists combos and account-pinned models

The service SHALL answer `GET /{surface}/v1/models` for each mounted surface (e.g. `GET /openai/v1/models`, `GET /anthropic/v1/models`) with a list rendered in that surface's native format, in which every `id` is a model identifier that resolves successfully through the router on that surface. The listing and the resolver SHALL agree: no `id` returned by `GET /{surface}/v1/models` SHALL be rejected when sent to that surface's primary generation endpoint.

Each entry SHALL carry only fields tinyroute can honestly populate; provider-provenance fields (creation time, capabilities, token limits) SHALL be constant defaults rather than fabricated values. The endpoint SHALL accept only `GET`; other methods SHALL return `405 Method Not Allowed`. Errors SHALL be returned in that surface's native JSON error envelope. The un-namespaced `GET /v1/models` SHALL no longer be served.

Listed identifiers SHALL come only from declared combos and provider model
whitelists (surfaced in their `provider:model` and bare whitelist forms).
Declared combos SHALL be listed in their `combo:<name>` key form, and every
listed `combo:<name>` id SHALL resolve when sent to that surface's primary
generation endpoint.

#### Scenario: Every listed ID is usable on its surface

- **WHEN** the OpenAI surface's provider whitelist contains `gpt-4o` and no combo
  is named `gpt-4o`
- **AND** `GET /openai/v1/models` is requested
- **THEN** the response includes `openai:gpt-4o`
- **AND** every `id` in the response, when sent as the model to `POST /openai/v1/chat/completions`, resolves without `404`

#### Scenario: Combos are listed in key form

- **WHEN** a combo named `fallback-chain` is declared and enabled
- **AND** `GET /openai/v1/models` is requested
- **THEN** the response includes `combo:fallback-chain`
- **AND** sending `combo:fallback-chain` as the model to `POST /openai/v1/chat/completions` resolves without `404`

#### Scenario: Cross-dialect models are listed when translatable

- **WHEN** an OpenAI-dialect provider is configured with a whitelisted model
- **AND** a translator is registered for the Anthropic→OpenAI pair
- **AND** `GET /anthropic/v1/models` is requested
- **THEN** the response includes that model
- **AND** every such `id`, when sent as the model to `POST /anthropic/v1/messages`, resolves without `404`

#### Scenario: Each surface renders its native list shape

- **WHEN** `GET /openai/v1/models` is requested
- **THEN** entries use the OpenAI shape (`object: "model"`, `created`, `owned_by`)
- **WHEN** `GET /anthropic/v1/models` is requested
- **THEN** entries use the Anthropic shape (`type: "model"`, `display_name`, `created_at`) with constant defaults for fields tinyroute does not track

#### Scenario: Legacy model listing is gone

- **WHEN** `GET /v1/models` is requested
- **THEN** the response status is `404`

#### Scenario: Method restriction

- **WHEN** `POST /openai/v1/models` is requested
- **THEN** the response status is `405 Method Not Allowed`
