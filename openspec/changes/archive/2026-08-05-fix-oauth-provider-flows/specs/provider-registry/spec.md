## MODIFIED Requirements

### Requirement: Presets carry per-provider OAuth metadata

A preset MAY carry OAuth metadata (flow type, client id, client secret, authorize/token/device endpoints, scopes, refresh profile, callback host, callback path, extra authorize parameters, and device-header profile) referenced by name from provider entries. The callback host, callback path, and extra authorize parameters let a preset express a provider's exact authorize-request shape (for example a fixed registered redirect URI or provider-required query parameters) without provider-specific code in the shared flow. The device-header profile lets a device-code preset declare a header scheme (such as kimi's `X-Msh-*`) without hardcoding it. The flow type MAY be a dedicated value (`pkce`, `device_code`, or a provider-specific runner such as `qoder` or `trae`) so non-standard flows are dispatched to their own runners. A provider entry SHALL NOT need to repeat these constants; it SHALL reference the preset.

#### Scenario: A provider inherits OAuth constants from its preset

- **WHEN** a provider references an OAuth-capable preset by name
- **THEN** the resolver SHALL supply that preset's client id, endpoints, scopes, refresh profile, callback host, callback path, extra authorize parameters, and device-header profile to the credential
- **AND** the provider entry SHALL remain terse

#### Scenario: Only OAuth-capable presets are offered for login

- **WHEN** `tinyroute auth login` lists providers
- **THEN** only presets that declare OAuth metadata SHALL appear
- **AND** static-key-only presets SHALL NOT

#### Scenario: A preset with a registered redirect URI carries its host and path

- **WHEN** a provider-owned preset (such as codex) requires a specific registered redirect URI
- **THEN** the preset SHALL declare `callback_host` and `callback_path`
- **AND** a loopback-flexible preset (such as claude) MAY omit them

#### Scenario: A preset carries provider-required extra authorize parameters

- **WHEN** a preset's authorize endpoint requires additional fixed query parameters (such as codex's `codex_cli_simplified_flow` or cline's `client_type`)
- **THEN** the preset SHALL declare them as `extra_params`
- **AND** a preset whose authorize endpoint requires none MAY omit `extra_params`

#### Scenario: A device-code preset declares its header profile

- **WHEN** a device-code preset's endpoint requires device-identity headers (such as kimi's `X-Msh-*`)
- **THEN** the preset SHALL declare a `device_header_profile`
- **AND** a device-code preset that needs none MAY omit it

#### Scenario: A non-standard flow declares a dedicated flow type

- **WHEN** a provider's login is not OAuth authorize-code or RFC 8628 device code (such as qoder or trae)
- **THEN** the preset SHALL declare a provider-specific `flow_type`
- **AND** SHALL be dispatched to that provider's runner rather than the generic PKCE/device runners
