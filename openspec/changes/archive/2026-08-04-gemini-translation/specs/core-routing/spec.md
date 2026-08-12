## ADDED Requirements

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
