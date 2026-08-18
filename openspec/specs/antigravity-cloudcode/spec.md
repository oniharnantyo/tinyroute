# antigravity-cloudcode Specification

## Purpose

Defines the CloudCode transport for Antigravity providers, which wraps native Gemini requests in a CloudCode envelope, obtains project IDs via onboarding, and routes requests through CloudCode-specific endpoints while preserving OAuth authentication and using native Gemini response handling.

## Requirements

### Requirement: A CloudCode transport wraps native Gemini requests in the CloudCode envelope

A provider MAY declare `transport: "cloudcode"`. When set, the proxy SHALL send generate requests to the CloudCode backend instead of the standard dialect path: the outbound request SHALL be `POST {base_url}/v1internal:generateContent` with a body of the CloudCode envelope `{project, model, userAgent:"antigravity", requestType:"agent", requestId, request:{<native Gemini payload>}}`. The native Gemini payload SHALL be produced by the existing inbound-to-gemini translation. A provider without a `transport` field SHALL use the standard dialect-driven path unchanged.

#### Scenario: A non-streaming antigravity request is sent as a CloudCode envelope

- **WHEN** a request is routed to a provider declaring `transport: "cloudcode"` for model `gemini-3.6-flash-medium`
- **THEN** the outbound request SHALL be `POST {base_url}/v1internal:generateContent`
- **AND** the body SHALL be the CloudCode envelope with the native Gemini payload nested under `request`
- **AND** the envelope `model` field SHALL carry the target model

#### Scenario: A streaming antigravity request uses the SSE endpoint

- **WHEN** the inbound request is a streaming request routed to a cloudcode provider
- **THEN** the outbound request SHALL be `POST {base_url}/v1internal:streamGenerateContent?alt=sse`

#### Scenario: Providers without a transport field are unaffected

- **WHEN** a request is routed to a provider with no `transport` field
- **THEN** the proxy SHALL use the standard dialect-driven request path exactly as before

### Requirement: CloudCode generate requires a project ID obtained via onboarding

Before any CloudCode generate request, the adapter SHALL obtain a `cloudaicompanionProject` by calling `loadCodeAssist` (with `onboardUser` as fallback) at the CloudCode bootstrap endpoint, authenticated with the resolved OAuth access token. The resulting project ID SHALL be injected as the envelope `project` field. The project ID SHALL be cached keyed by access token for a bounded window and reused across requests without re-onboarding until it expires or the token changes.

#### Scenario: First request triggers onboarding and injects the project ID

- **WHEN** a cloudcode generate is attempted with no cached project ID for the current access token
- **THEN** the adapter SHALL call `loadCodeAssist` to obtain the project ID
- **AND** SHALL inject it as the envelope `project` field
- **AND** SHALL cache it for the current access token

#### Scenario: Subsequent requests reuse the cached project ID

- **WHEN** a cloudcode generate is attempted and a valid cached project ID exists for the current access token
- **THEN** the adapter SHALL NOT call `loadCodeAssist` again
- **AND** SHALL reuse the cached project ID

### Requirement: CloudCode requests carry IDE fingerprint headers

A CloudCode outbound request SHALL carry `Authorization: Bearer <access token>` and an Antigravity IDE `User-Agent` (for example `antigravity/ide/2.1.1 darwin/arm64`). The OAuth token SHALL be resolved via the provider's existing OAuth refresh credential; no new authentication flow SHALL be introduced for the runtime path.

#### Scenario: Antigravity IDE User-Agent and Bearer token are sent

- **WHEN** a cloudcode request is built
- **THEN** the request SHALL include `Authorization: Bearer <token>` and the Antigravity IDE `User-Agent`

### Requirement: The antigravity preset targets the CloudCode transport

The `antigravity` preset SHALL declare `transport: "cloudcode"` and CloudCode runtime/bootstrap endpoints (`daily-cloudcode-pa.googleapis.com` and `cloudcode-pa.googleapis.com`), not the generic Gemini API. The preset's existing OAuth/PKCE metadata (client id, scopes, authorize/token endpoints) SHALL remain unchanged, and existing stored OAuth credentials SHALL remain valid without re-authentication.

#### Scenario: Adding antigravity wires the CloudCode transport

- **WHEN** a user adds the antigravity provider from its preset
- **THEN** the resulting provider entry SHALL declare `transport: "cloudcode"` and a CloudCode base_url
- **AND** the OAuth login SHALL behave exactly as before this change

### Requirement: CloudCode responses are relayed as native Gemini

A CloudCode generate response (streaming SSE or non-streaming JSON) SHALL be in native Gemini format and SHALL be relayed and translated using the existing gemini-dialect response handling, including usage scanning.

#### Scenario: A streaming CloudCode response is relayed as native Gemini SSE

- **WHEN** a cloudcode streaming generate returns native Gemini SSE chunks
- **THEN** the response SHALL be relayed and translated using the gemini dialect's response handling

### Requirement: The dashboard model probe exercises the CloudCode path

A dashboard model probe of a cloudcode provider's model SHALL flow through the CloudCode transport (onboarding plus envelope) and SHALL report the real upstream status, succeeding when the upstream serves the model rather than failing at routing or with a generic-endpoint 404.

#### Scenario: Probing an antigravity model reaches the CloudCode backend

- **WHEN** a user tests a model on a cloudcode provider via the dashboard
- **THEN** the probe SHALL invoke the CloudCode transport
- **AND** SHALL report success when the upstream returns a valid response for that model
