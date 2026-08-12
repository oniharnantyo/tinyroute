## ADDED Requirements

### Requirement: A native Gemini dialect translates requests and responses

A `gemini` dialect SHALL implement the dialect contract (`Paths`, `RewriteModel`, `AuthHeaders`, `InjectUsageOption`, `NewUsageScanner`, `WriteError`) for Google's native `generateContent` protocol, translating between an OpenAI or Anthropic inbound surface and the native Gemini request/response shapes. It SHALL coexist with the existing `anthropic` and `openai` dialects.

#### Scenario: A request on the OpenAI surface is translated to native Gemini

- **WHEN** a request arrives on the OpenAI surface for a provider using the `gemini` dialect
- **THEN** the outbound request body SHALL be a native Gemini `generateContent` payload
- **AND** the response SHALL be translated back to the OpenAI response shape

#### Scenario: Streaming is relayed

- **WHEN** the inbound request is a streaming request
- **THEN** the gemini dialect SHALL relay the native stream and translate usage events for observation

### Requirement: The Gemini dialect authenticates with either API key or OAuth

The `gemini` dialect's `AuthHeaders` SHALL authenticate with a static API key when one is configured and with `Authorization: Bearer` when an OAuth access token is resolved, following the same credential-aware rules as the other dialects.

#### Scenario: API-key authentication

- **WHEN** a gemini-dialect provider uses a static API key
- **THEN** the outbound request SHALL authenticate using the API key in the Gemini-native location

#### Scenario: OAuth authentication

- **WHEN** a gemini-dialect provider resolves an OAuth access token
- **THEN** the outbound request SHALL carry `Authorization: Bearer <token>`
