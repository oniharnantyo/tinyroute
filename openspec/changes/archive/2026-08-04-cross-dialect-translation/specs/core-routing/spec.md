## MODIFIED Requirements

### Requirement: Resolution is faithful to the inbound surface

For a request received on a given surface, the router SHALL resolve to a provider whose
dialect matches that surface, OR to a provider of a different dialect for which a
cross-dialect translator is registered. A model reference whose resolved provider speaks a
different dialect for which NO translator is registered SHALL be rejected with an error
naming the mismatch, rather than relayed to a provider that would reject the request body.

The router SHALL determine translator availability through a single predicate supplied at
construction time, so that both resolution and model discovery agree on what is
cross-dialect-reachable.

#### Scenario: Same-dialect prefix resolves

- **WHEN** a request on the OpenAI surface specifies model `openai:gpt-4o`
- **AND** an `openai`-dialect provider whitelists `gpt-4o`
- **THEN** the request resolves and is proxied to that provider

#### Scenario: Cross-dialect prefix resolves when a translator exists

- **WHEN** a request on the Anthropic surface specifies an OpenAI-dialect provider's whitelisted model
- **AND** a translator is registered for the Anthropic→OpenAI pair
- **THEN** the request resolves to that provider and is proxied with translation

#### Scenario: Cross-dialect prefix is rejected when no translator exists

- **WHEN** a request on the OpenAI surface specifies a provider whose dialect has no registered translator to OpenAI
- **THEN** the router rejects the request with a clear error naming the dialect mismatch
- **AND** no upstream request is made

### Requirement: Model discovery lists only resolvable IDs

The service SHALL answer `GET /{surface}/v1/models` for each mounted surface (e.g. `GET /openai/v1/models`, `GET /anthropic/v1/models`) with a list rendered in that surface's native format, in which every `id` is a model identifier that resolves successfully through the router on that surface. The listing and the resolver SHALL agree: no `id` returned by `GET /{surface}/v1/models` SHALL be rejected when sent to that surface's primary generation endpoint.

Each entry SHALL carry only fields tinyroute can honestly populate; provider-provenance fields (creation time, capabilities, token limits) SHALL be constant defaults rather than fabricated values. The endpoint SHALL accept only `GET`; other methods SHALL return `405 Method Not Allowed`. Errors SHALL be returned in that surface's native JSON error envelope. The un-namespaced `GET /v1/models` SHALL no longer be served.

#### Scenario: Every listed ID is usable on its surface

- **WHEN** the OpenAI surface's provider whitelist contains `gpt-4o` and no explicit route matches the bare name `gpt-4o`
- **AND** `GET /openai/v1/models` is requested
- **THEN** the response includes `openai:gpt-4o`
- **AND** every `id` in the response, when sent as the model to `POST /openai/v1/chat/completions`, resolves without `404`

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

## ADDED Requirements

### Requirement: Cross-dialect translation

When an inbound request's surface dialect differs from the resolved provider's dialect, the proxy SHALL translate the request body into the provider's dialect before sending it, and SHALL translate the provider's response (streaming and non-streaming) back into the surface dialect before relaying it to the client. When the two dialects match, the proxy SHALL pass the body and response through unchanged.

Translators SHALL be resolved through a registry that pivots through OpenAI as the canonical intermediate format: a translator registered for an exact source→target pair SHALL run as a direct route; otherwise the registry SHALL compose source→OpenAI→target. A pair with no registered path SHALL cause the hop to be treated as non-translatable (and thus rejected at resolution time).

The streaming translator SHALL be stateful: it SHALL maintain the ordering of the surface dialect's content blocks across upstream chunks, synthesize any identifiers the surface dialect requires, and emit closing frames when the upstream stream terminates. The same translation entry point SHALL accept a sentinel empty chunk at end-of-stream to drain pending state.

#### Scenario: Cross-dialect request is translated and proxied

- **WHEN** a client sends an Anthropic-format request to `/anthropic/v1/messages`
- **AND** the resolved provider speaks the OpenAI dialect
- **THEN** the outbound request body is in OpenAI chat-completions shape
- **AND** the outbound request is sent to the provider's `/v1/chat/completions` path
- **AND** the client receives an Anthropic-format response

#### Scenario: Streaming response is translated event-by-event

- **WHEN** a streaming cross-dialect response is relayed
- **THEN** each upstream chunk is translated into one or more surface-dialect SSE events
- **AND** the emitted event sequence is valid for the surface dialect (correct event ordering, block indices, and synthesized identifiers)
- **AND** a closing `message_stop` (or equivalent) is emitted when the upstream stream terminates

#### Scenario: Streaming usage is reported with a known fidelity gap

- **WHEN** an Anthropic-surface streaming response is produced from an OpenAI provider
- **THEN** the `message_start` event carries placeholder zero usage
- **AND** the true token usage, when reported by the upstream final chunk, is attached to the `message_delta` event at stream end

#### Scenario: Non-streaming response is translated as a whole

- **WHEN** a non-streaming cross-dialect response is relayed
- **THEN** the full provider response body is translated into a single surface-dialect response body before being written to the client

#### Scenario: Unmappable fields are dropped, not fatal

- **WHEN** a request or response field has no equivalent in the target dialect
- **THEN** the field is omitted from the translated body
- **AND** the request is not rejected solely because of that field
- **AND** the omission is recorded at debug log level
