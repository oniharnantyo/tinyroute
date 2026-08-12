## MODIFIED Requirements

### Requirement: Model discovery lists only resolvable IDs

The service SHALL answer `GET /{surface}/v1/models` for each mounted surface (e.g.
`GET /openai/v1/models`, `GET /anthropic/v1/models`) with a list rendered in that
surface's native format, in which every `id` is a model identifier that resolves
successfully through the router on that surface. The listing and the resolver SHALL
agree: no `id` returned by `GET /{surface}/v1/models` SHALL be rejected when sent to
that surface's primary generation endpoint.

Each entry SHALL carry only fields tinyroute can honestly populate; provider-provenance
fields (creation time, capabilities, token limits) SHALL be constant defaults rather
than fabricated values. The endpoint SHALL accept only `GET`; other methods SHALL
return `405 Method Not Allowed`. Errors SHALL be returned in that surface's native
JSON error envelope. The un-namespaced `GET /v1/models` SHALL no longer be served.

#### Scenario: Every listed ID is usable on its surface

- **WHEN** the OpenAI surface's provider whitelist contains `gpt-4o` and no explicit route matches the bare name `gpt-4o`
- **AND** `GET /openai/v1/models` is requested
- **THEN** the response includes `openai:gpt-4o`
- **AND** every `id` in the response, when sent as the model to `POST /openai/v1/chat/completions`, resolves without `404`

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

## ADDED Requirements

### Requirement: Inbound endpoints are namespaced per dialect

Each dialect SHALL mount its inbound endpoints under a namespaced surface
(`/{vendor}/v1/*`), distinct from the outbound upstream paths it uses to call
providers. The outbound upstream path SHALL remain the provider's canonical API path
(e.g. `/v1/chat/completions`) and SHALL NOT be derived from the inbound namespace.
The legacy un-namespaced inbound paths (`/v1/chat/completions`, `/v1/messages`)
SHALL no longer be served.

#### Scenario: OpenAI chat is namespaced inbound, canonical outbound

- **WHEN** a client sends an OpenAI chat request to `/openai/v1/chat/completions`
- **THEN** the request is served by the OpenAI dialect
- **AND** the outbound request to the upstream provider uses `/v1/chat/completions` (not `/openai/v1/chat/completions`)

#### Scenario: Anthropic messages are namespaced

- **WHEN** a client sends an Anthropic request to `/anthropic/v1/messages`
- **THEN** the request is served by the Anthropic dialect

#### Scenario: Legacy inbound paths are gone

- **WHEN** `POST /v1/chat/completions` is requested
- **THEN** the response status is `404`

### Requirement: Resolution is faithful to the inbound surface

For a request received on a given surface, the router SHALL resolve only to
providers whose dialect matches that surface's dialect. A model reference whose
resolved provider speaks a different dialect SHALL be rejected with an error naming
the mismatch, rather than relayed to a provider that would reject the request body.
This constraint holds until a cross-dialect translator is provided.

#### Scenario: Same-dialect prefix resolves

- **WHEN** a request on the OpenAI surface specifies model `openai:gpt-4o`
- **AND** an `openai`-dialect provider whitelists `gpt-4o`
- **THEN** the request resolves and is proxied to that provider

#### Scenario: Cross-dialect prefix is rejected

- **WHEN** a request on the OpenAI surface specifies model `anthropic:claude-3-5-sonnet`
- **THEN** the router rejects the request with a clear error
- **AND** no upstream request is made

### Requirement: OpenAI Responses surface

The service SHALL serve `POST /openai/v1/responses` via a dedicated `openai-responses`
dialect. The dialect SHALL route by the request's `model` field, rewrite the model to
the resolved hop's target model, relay the response (including streaming SSE), and
extract usage from the `response.completed` event. The endpoint SHALL behave as a
thin pass-through: server-side stateful features (`store`, `previous_response_id`,
`conversation`, `background`, and built-in tools) are not provided by tinyroute, and
their semantics across provider failover are not guaranteed.

#### Scenario: Responses request routes by model and relays

- **WHEN** a client sends `POST /openai/v1/responses` with `model: openai-responses:gpt-5.1`
- **AND** an `openai-responses`-dialect provider whitelists `gpt-5.1`
- **THEN** the request is proxied to that provider at `/v1/responses`
- **AND** the response is relayed to the client in the Responses shape

#### Scenario: Streaming usage is extracted from the completion event

- **WHEN** a streaming `POST /openai/v1/responses` request is relayed
- **THEN** usage is extracted from the `response.completed` SSE event
- **AND** reasoning tokens, when reported upstream, are captured

#### Scenario: Stateful features are pass-through only

- **WHEN** a Responses request uses `store`, `previous_response_id`, or a built-in tool
- **THEN** the request is relayed unchanged to the resolved provider
- **AND** tinyroute provides no server-side storage, conversation continuity, or tool execution
