## Purpose

Defines core routing rules, model resolution, surface namespacing, and model discovery.

## Requirements

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

### Requirement: Inbound endpoints are namespaced per dialect

Each dialect SHALL mount its inbound endpoints under a namespaced surface (`/{vendor}/v1/*`), distinct from the outbound upstream paths it uses to call providers. The outbound upstream path SHALL remain the provider's canonical API path (e.g. `/v1/chat/completions`) and SHALL NOT be derived from the inbound namespace. The legacy un-namespaced inbound paths (`/v1/chat/completions`, `/v1/messages`) SHALL no longer be served.

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

For a request received on a given surface, the router SHALL resolve to a provider whose dialect matches that surface, OR to a provider of a different dialect for which a cross-dialect translator is registered. A model reference whose resolved provider speaks a different dialect for which NO translator is registered SHALL be rejected with an error naming the mismatch, rather than relayed to a provider that would reject the request body.

The router SHALL determine translator availability through a single predicate supplied at construction time, so that both resolution and model discovery agree on what is cross-dialect-reachable.

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

### Requirement: OpenAI Responses surface

The service SHALL serve `POST /openai/v1/responses` via a dedicated `openai-responses` dialect. The dialect SHALL route by the request's `model` field, rewrite the model to the resolved hop's target model, relay the response (including streaming SSE), and extract usage from the `response.completed` event. The endpoint SHALL behave as a thin pass-through: server-side stateful features (`store`, `previous_response_id`, `conversation`, `background`, and built-in tools) are not provided by tinyroute, and their semantics across provider failover are not guaranteed.

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

### Requirement: Dialects emit the correct auth shape for OAuth tokens

A dialect's `AuthHeaders` SHALL distinguish a static API key from an OAuth access token. An OAuth access token SHALL be sent as `Authorization: Bearer <token>`. The inbound caller's credential SHALL never be forwarded upstream; outbound auth SHALL derive solely from the resolved provider credential.

#### Scenario: An OAuth token is sent as Bearer

- **WHEN** a hop resolves an OAuth access token for a provider
- **THEN** the outbound request SHALL carry `Authorization: Bearer <token>`

#### Scenario: The inbound credential is never forwarded

- **WHEN** a request is proxied to an upstream provider
- **THEN** the outbound Authorization header SHALL be set exclusively from the provider's resolved credential
- **AND** SHALL NOT contain the inbound caller's `tr_live_` key

### Requirement: The anthropic dialect falls back from x-api-key to Bearer for OAuth

The anthropic dialect SHALL send `x-api-key` when authenticating with a static API key, and SHALL send `Authorization: Bearer` when authenticating with an OAuth access token. This enables OAuth-subscriber providers (Claude, GitHub Copilot on the anthropic surface, Kimi's anthropic surface) without changing static-key behavior.

#### Scenario: Static key uses x-api-key

- **WHEN** an anthropic-dialect hop authenticates with a static API key
- **THEN** the outbound request SHALL carry `x-api-key: <key>`
- **AND** SHALL NOT carry an OAuth `Authorization: Bearer` header from the credential

#### Scenario: OAuth token uses Bearer

- **WHEN** an anthropic-dialect hop authenticates with an OAuth access token
- **THEN** the outbound request SHALL carry `Authorization: Bearer <token>`
- **AND** SHALL carry the `anthropic-version` header as before

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

### Requirement: Gemini dialect translation

The service SHALL translate between the OpenAI and Gemini dialects so that a Gemini- or Vertex-dialect provider is reachable from the OpenAI surface directly, and from the Anthropic surface via the cross-dialect registry's two-hop pivot through OpenAI. The translation SHALL cover request bodies, non-streaming responses, and streaming responses, using the registry and proxy seams defined by the cross-dialect translation requirement.

The Gemini translators SHALL preserve interoperability across the dialect's structural differences: function names outside the Gemini charset SHALL be sanitized on the request and restored to their original form on the response; tool calls and tool results SHALL be mapped between OpenAI `tool_calls`/`role:"tool"` and Gemini `functionCall`/`functionResponse` (including recovering the function name for a tool result from the preceding call); and consecutive same-role turns SHALL be merged where the Gemini dialect requires it.

#### Scenario: Gemini provider is reachable from the OpenAI surface

- **WHEN** a client sends an OpenAI chat request whose resolved provider speaks the Gemini dialect
- **THEN** the outbound request body is in Gemini `contents`/`generationConfig` shape
- **AND** the client receives an OpenAI-format response

#### Scenario: Gemini provider is reachable from the Anthropic surface via two-hop

- **WHEN** a client sends an Anthropic request whose resolved provider speaks the Gemini dialect
- **AND** no direct Anthropic→Gemini translator is registered
- **THEN** the request is translated Anthropic→OpenAI→Gemini
- **AND** the response is translated Gemini→OpenAI→Anthropic
- **AND** the client receives a valid Anthropic-format response

#### Scenario: Function names survive the Gemini charset

- **WHEN** a tool name contains characters outside the Gemini function-name charset
- **THEN** the outbound request uses a sanitized name acceptable to the Gemini provider
- **AND** the function name restored to the client on the response matches the original

#### Scenario: Tool calls and results are structurally mapped

- **WHEN** a request carries OpenAI tool calls and tool results destined for a Gemini provider
- **THEN** the outbound request uses Gemini `functionCall` parts for calls
- **AND** uses Gemini `functionResponse` parts for results, each carrying the recovered function name
- **AND** non-object tool results are wrapped so the Gemini `response.result` field is an object

#### Scenario: Multi-turn thinking uses a placeholder signature

- **WHEN** an assistant turn carrying reasoning or tool calls is translated toward a Gemini provider
- **THEN** the outbound turn carries a constant placeholder `thoughtSignature`
- **AND** the request is not rejected for a missing signature

#### Scenario: Consecutive same-role turns are merged

- **WHEN** translation would produce two adjacent Gemini contents with the same role
- **THEN** the outbound request merges them into a single content with combined parts

### Requirement: Cross-dialect translation applies to combo members

Each combo member SHALL be treated like an ordinary hop for translation: a member
whose provider dialect differs from the surface SHALL require a registered
translator, exactly as a non-combo hop does.

#### Scenario: Combo member requiring translation resolves only when translatable

- **WHEN** a combo member's provider dialect differs from the surface and no translator is registered
- **THEN** the combo SHALL be rejected by validation/resolution with a clear error

