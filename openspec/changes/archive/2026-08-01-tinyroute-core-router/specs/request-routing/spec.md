## ADDED Requirements

### Requirement: The service exposes both inbound dialect surfaces plus model discovery

The HTTP server SHALL accept `POST /v1/messages` for the Anthropic Messages dialect and
`POST /v1/chat/completions` for the OpenAI dialect. Each inbound path SHALL be owned by its dialect
implementation rather than hard-coded in the server. The server SHALL also answer `GET /v1/models`
with an OpenAI-shaped model list synthesized from configured routes and chains. Unknown paths SHALL
return `404`.

#### Scenario: Claude Code reaches the Anthropic surface

- **WHEN** a client sends `POST /v1/messages` with a valid key
- **THEN** the request MUST be handled using the Anthropic dialect

#### Scenario: OpenAI-compatible client reaches its surface

- **WHEN** a client sends `POST /v1/chat/completions` with a valid key
- **THEN** the request MUST be handled using the OpenAI dialect

#### Scenario: Model list is synthesized from configuration

- **WHEN** `GET /v1/models` is requested
- **THEN** the response MUST list concrete model names drawn from route patterns and chain hops
- **AND** MUST NOT require a hard-coded model catalog

#### Scenario: Adding a dialect adds its own paths

- **WHEN** a new dialect implementation declaring its own inbound paths is registered
- **THEN** those paths MUST become routable without editing the server or the proxy orchestrator

### Requirement: Only the fields required for routing are interpreted

The proxy SHALL decode the request body only far enough to read the requested model and whether the
response is streamed. All other fields MUST be preserved byte-for-byte when the request is forwarded.
Rewriting the model name MUST NOT drop, reorder-sensitively alter, or reject fields the service does
not recognize.

#### Scenario: Unrecognized fields survive forwarding

- **WHEN** a request body contains a parameter the service has no knowledge of
- **THEN** the forwarded request MUST still contain that parameter unchanged

#### Scenario: Model is rewritten for a concrete hop

- **WHEN** a chain hop specifies a concrete model different from the requested one
- **THEN** the forwarded body's `model` field MUST be the hop's model
- **AND** every other field MUST be unchanged

#### Scenario: Oversized body is rejected

- **WHEN** a request body exceeds the 32 MB buffer limit
- **THEN** the service MUST reject the request with an error in the inbound dialect's native format
- **AND** MUST NOT attempt to forward it

### Requirement: Requests are attempted against chain hops in order, skipping unhealthy providers

The proxy SHALL attempt hops in the order declared by the resolved chain. A hop whose provider is in
an active cooldown window SHALL be skipped without a network attempt. When every hop is skipped or
exhausted, the proxy SHALL return an error in the inbound dialect's native format describing the
attempts made.

#### Scenario: Second hop serves after the first fails

- **WHEN** the first hop returns `503` before any response byte and the second hop succeeds
- **THEN** the client MUST receive the second hop's response
- **AND** the recorded attempt list MUST contain both hops with their statuses

#### Scenario: Provider in cooldown is skipped without a request

- **WHEN** a provider is in an active cooldown window
- **THEN** no network request MUST be made to that provider
- **AND** the next hop MUST be attempted

#### Scenario: Chain exhausted

- **WHEN** every hop fails or is skipped
- **THEN** the response MUST be a native-format error for the inbound dialect summarizing the attempts

### Requirement: Failover is only permitted before the first response byte reaches the client

Once any byte of a provider's response body has been written to the client, the proxy MUST NOT attempt
another hop. A failure occurring after that point SHALL be propagated to the client and recorded as a
mid-stream failure. The proxy MUST NOT buffer complete responses in order to preserve the option to
fail over.

#### Scenario: Failure before commit falls over

- **WHEN** a provider's connection fails before any response byte has been relayed
- **THEN** the next hop MUST be attempted

#### Scenario: Failure after commit is propagated

- **WHEN** a provider begins streaming, bytes have been relayed, and the upstream connection then dies
- **THEN** no further hop MUST be attempted
- **AND** the truncated stream MUST be propagated to the client
- **AND** the record MUST mark the outcome as a mid-stream failure

#### Scenario: Streaming is not buffered

- **WHEN** a streamed response is relayed
- **THEN** chunks MUST be flushed to the client as they arrive rather than accumulated until completion

### Requirement: Failure responses are classified to decide retry and cooldown

The proxy SHALL classify each failed attempt and apply the following behavior. Cooldown durations
SHALL default from deployment settings and MAY be overridden per provider.

| Condition | Retry next hop | Cooldown |
|---|---|---|
| Connection error or response-header timeout | yes | `TINYROUTE_COOLDOWN_5XX`, escalating on repeat |
| `429` | yes | `Retry-After` when present, else `TINYROUTE_COOLDOWN_429` |
| `5xx` | yes | `TINYROUTE_COOLDOWN_5XX` |
| `404` | yes | none |
| `401` or `403` | no | 15 minutes, with a warning surfaced in CLI status |
| Other `4xx` | no | none |

#### Scenario: Rate limit honors Retry-After

- **WHEN** a provider returns `429` with `Retry-After: 30`
- **THEN** the next hop MUST be attempted
- **AND** that provider MUST be cooled down for 30 seconds

#### Scenario: Missing model falls over without penalty

- **WHEN** a provider returns `404` for a model that is not available there
- **THEN** the next hop MUST be attempted
- **AND** no cooldown MUST be applied to that provider

#### Scenario: Authentication failure does not consume the chain

- **WHEN** a provider returns `401`
- **THEN** no further hop MUST be attempted
- **AND** that provider MUST be cooled down for 15 minutes
- **AND** the condition MUST be surfaced in `tinyroute status`

#### Scenario: Malformed request is returned unchanged

- **WHEN** a provider returns `400` for a request the client formed incorrectly
- **THEN** no further hop MUST be attempted
- **AND** the provider's error body MUST be recorded so the cause is inspectable

### Requirement: Provider health state survives restarts

Cooldown state SHALL be persisted to `state.json` so that a provider known to be rate-limited is not
retried immediately after a daemon restart. Expired cooldowns SHALL be treated as absent.

#### Scenario: Cooldown persists across restart

- **WHEN** a provider is cooled down for 60 seconds and the daemon restarts after 10 seconds
- **THEN** that provider MUST still be skipped for the remaining window

#### Scenario: Expired cooldown is ignored

- **WHEN** a persisted cooldown window has already elapsed
- **THEN** the provider MUST be considered available

### Requirement: Errors are returned in the inbound client's native format

When the proxy itself produces an error response, the body SHALL use the error envelope of the inbound
dialect rather than a gateway-specific shape, so that clients parse and display it correctly. This
applies to authentication failures, rate-limit rejections, no-matching-route, oversized bodies, and
chain exhaustion.

#### Scenario: Anthropic client receives an Anthropic error envelope

- **WHEN** a request on `/v1/messages` exhausts its chain
- **THEN** the response body MUST use the Anthropic error format

#### Scenario: OpenAI client receives an OpenAI error envelope

- **WHEN** a request on `/v1/chat/completions` exhausts its chain
- **THEN** the response body MUST use the OpenAI error format

### Requirement: Outbound credentials are presented in the form each dialect requires

The dialect of the target provider SHALL determine default credential headers: the Anthropic dialect
SHALL send an API-key header together with a version header, and the OpenAI dialect SHALL send a
bearer authorization header. A provider's configured `headers` map SHALL override or remove any
default header. The inbound caller's own credential MUST NOT be forwarded upstream.

#### Scenario: Anthropic-dialect provider receives its expected headers

- **WHEN** a hop targets a provider with dialect `anthropic`
- **THEN** the outbound request MUST carry the API-key and version headers for that dialect

#### Scenario: Configured header removes a default

- **WHEN** a provider's `headers` map sets the default authorization header to null
- **THEN** the outbound request MUST NOT include that header

#### Scenario: Inbound credential is not leaked upstream

- **WHEN** a client authenticates with a tinyroute-issued key
- **THEN** that key MUST NOT appear in any outbound request to a provider

### Requirement: Timeouts bound connection establishment without truncating long streams

The outbound HTTP transport SHALL apply a response-header timeout, which also defines the window in
which failover remains possible. A whole-request deadline MUST NOT be configured, so that long
streaming generations are not severed mid-response.

#### Scenario: Slow header response fails over

- **WHEN** a provider does not return response headers within the response-header timeout
- **THEN** the attempt MUST be treated as a pre-commit failure and the next hop MUST be attempted

#### Scenario: Long stream is not severed

- **WHEN** a provider streams a response for longer than the response-header timeout after headers
  have been received
- **THEN** the stream MUST continue uninterrupted by any client-side deadline
