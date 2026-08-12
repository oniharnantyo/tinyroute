## ADDED Requirements

### Requirement: Model discovery lists only resolvable IDs

The service SHALL answer `GET /v1/models` with an OpenAI-shaped list (`object: "list"`, each entry
`object: "model"`) in which every `id` is a model identifier that resolves successfully through the
router on the OpenAI surface. The listing and the resolver SHALL agree: no `id` returned by
`GET /v1/models` SHALL be rejected when sent to `POST /v1/chat/completions`.

Each entry SHALL carry a constant `created` of `0` and a constant `owned_by` of `"tinyroute"`. The
endpoint SHALL accept only `GET`; other methods SHALL return `405 Method Not Allowed`. Errors SHALL
be returned in the OpenAI JSON error envelope used by the chat endpoint.

#### Scenario: Every listed ID is usable

- **WHEN** a provider whitelist contains `gpt-4o` and no explicit manual route matches the bare name `gpt-4o`
- **AND** `GET /v1/models` is requested
- **THEN** the response includes `openai:gpt-4o`
- **AND** does NOT include the bare `gpt-4o`
- **AND** every `id` in the response, when sent as the model to `POST /v1/chat/completions`, resolves without `404`

#### Scenario: A bare ID is listed only when a route makes it resolvable

- **WHEN** a manual route matches the bare model `fast` on the OpenAI surface
- **THEN** `GET /v1/models` includes `fast`

#### Scenario: Stable fields across requests

- **WHEN** `GET /v1/models` is requested twice
- **THEN** every entry has `created` equal to `0` and `owned_by` equal to `"tinyroute"` in both responses

#### Scenario: Method restriction

- **WHEN** `POST /v1/models` is requested
- **THEN** the response status is `405 Method Not Allowed`

#### Scenario: Errors use the JSON envelope

- **WHEN** the router cannot be built from the current topology
- **THEN** the response is a JSON error object in the OpenAI envelope, not plain text