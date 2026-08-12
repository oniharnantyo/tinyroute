## ADDED Requirements

### Requirement: Providers are declared as data in config.json

`config.json` SHALL declare providers as a JSON object keyed by provider name. Each provider entry
SHALL carry a `dialect`, a `base_url`, an optional `api_key`, and an optional `headers` map. A
provider MUST NOT require any Go code to be added, modified, or registered.

#### Scenario: A provider is usable from configuration alone

- **WHEN** a provider entry with a valid `dialect`, `base_url`, and `api_key` is added to
  `config.json` and referenced from a route chain
- **THEN** requests MUST be routable to it without any change to the compiled binary

#### Scenario: Unknown dialect is rejected

- **WHEN** a provider declares `"dialect": "cohere"` and no such dialect is registered
- **THEN** validation MUST fail naming the provider, the unknown dialect, and the registered dialects

#### Scenario: Custom headers override dialect defaults

- **WHEN** a provider declares `"headers": {"api-key": "…", "Authorization": null}`
- **THEN** outbound requests MUST include the `api-key` header
- **AND** the dialect's default `Authorization` header MUST be omitted

### Requirement: Routes map an inbound surface and model pattern to an ordered chain

`config.json` SHALL declare routes as an ordered array. Each route SHALL carry `from` identifying the
inbound dialect, `match` as a glob against the requested model name, and `chain` as an ordered list of
`provider:model` hops. The literal `$model` in a hop SHALL forward the client's requested model name
unchanged. Routes SHALL be evaluated in declaration order and the first route matching both `from`
and `match` SHALL be selected.

#### Scenario: First matching route wins

- **WHEN** routes declare `claude-*` before `*` for `from: anthropic` and a request asks for
  `claude-sonnet-4-6`
- **THEN** the `claude-*` route's chain MUST be used

#### Scenario: Surface disambiguates identical globs

- **WHEN** one route declares `from: anthropic, match: *` and another declares `from: openai, match: *`
- **AND** a request arrives on `/v1/chat/completions` for model `claude-sonnet-4-6`
- **THEN** the `from: openai` route MUST be selected

#### Scenario: Model passthrough token

- **WHEN** a chain hop is `anthropic:$model` and the client requested `claude-opus-4-1`
- **THEN** the outbound request MUST target `claude-opus-4-1` at provider `anthropic`

#### Scenario: No route matches

- **WHEN** no route matches the request's surface and model
- **THEN** the response MUST be an error in the inbound dialect's native error format stating that no
  route matched, and MUST NOT be a generic gateway error

### Requirement: Configuration is strict JSON with uniform variable interpolation

`config.json` MUST be parsed as strict JSON; comments MUST NOT be supported. Every string value in the
file SHALL support `${VAR}` interpolation resolved from the process environment at load time. A single
interpolation mechanism SHALL apply to all string values, including `base_url` and `headers`; no
field-specific credential indirection SHALL be added.

#### Scenario: Comments are rejected

- **WHEN** `config.json` contains a `//` comment line
- **THEN** loading MUST fail with a JSON syntax error identifying the location

#### Scenario: Interpolation applies outside api_key

- **WHEN** a provider declares `"base_url": "${AZURE_ENDPOINT}"` and that variable is set
- **THEN** the resolved base URL MUST be the variable's value

#### Scenario: Unset reference is reported distinctly from an empty value

- **WHEN** a provider declares `"api_key": "${ZAI_API_KEY}"` and the variable is unset
- **THEN** validation MUST report the variable as unset, naming both the provider and the variable

### Requirement: config.json is secret-bearing and protected on disk

Because provider credentials are stored inline, `config.json` SHALL be treated as a secret file. The
service SHALL verify permissions on load and warn when the file is group- or world-readable. `init`
SHALL create the file with mode `0600`. When `init` detects a `.git` directory in the target
directory tree, it SHALL add the config file to `.gitignore` before writing it.

#### Scenario: Loose permissions produce a warning

- **WHEN** `config.json` has mode `0644` at load time
- **THEN** a warning naming the file and its mode MUST be emitted
- **AND** startup MUST continue

#### Scenario: Created with restrictive permissions

- **WHEN** `tinyroute init` creates `config.json`
- **THEN** the file mode MUST be `0600`

#### Scenario: Git repository detected

- **WHEN** `tinyroute init` runs where a `.git` directory is present
- **THEN** the config file path MUST be added to `.gitignore` before the config is written
- **AND** the action MUST be reported to the user

### Requirement: Configuration is hot-reloaded and validated before taking effect

The service SHALL detect changes to `config.json` by comparing modification time before serving a
request, and SHALL reload and validate the file when it has changed. A valid snapshot SHALL replace
the active snapshot atomically. An invalid snapshot MUST be rejected, the previously active snapshot
MUST continue serving traffic, and the failure MUST be logged. An in-flight request SHALL continue
using the snapshot it resolved against.

#### Scenario: Valid edit takes effect without restart

- **WHEN** a new provider is added to `config.json` and a request arrives
- **THEN** the new provider MUST be available for routing without restarting the daemon

#### Scenario: Invalid edit does not disrupt service

- **WHEN** `config.json` is edited to contain malformed JSON
- **THEN** the daemon MUST continue serving using the last valid snapshot
- **AND** MUST log an error identifying the parse failure
- **AND** MUST NOT exit

#### Scenario: In-flight request keeps its snapshot

- **WHEN** a request has resolved a chain and `config.json` changes before that request completes
- **THEN** the in-flight request MUST continue using its originally resolved chain

### Requirement: Machine writes to config.json are atomic

Commands that modify `config.json` SHALL write to a temporary file created with mode `0600` in the
same directory and then rename it into place. Rewrites SHALL use canonical formatting with two-space
indentation and sorted object keys so that diffs remain readable. A partially written file MUST never
be observable by the reload mechanism.

#### Scenario: Concurrent reload never sees a partial file

- **WHEN** `tinyroute add` writes the config while the daemon is running
- **THEN** the daemon MUST observe either the previous complete file or the new complete file
- **AND** MUST NOT report a parse error caused by a partial write

#### Scenario: Canonical formatting on rewrite

- **WHEN** a command rewrites `config.json`
- **THEN** the output MUST use two-space indentation with object keys in sorted order

### Requirement: Presets scaffold configuration and are never consulted at request time

The binary SHALL embed a preset table of known providers containing dialect, base URL, and the
conventional credential variable name. Presets SHALL be read only by `init` and `add`. The request
path MUST resolve providers exclusively from `config.json`. A provider that requires behavior the
service does not implement MUST NOT be given a preset, and no provider-specific code path SHALL be
added to accommodate it.

#### Scenario: Preset produces a plain config entry

- **WHEN** `tinyroute add zai` is run
- **THEN** a provider entry for `zai` with dialect `anthropic` and its base URL MUST be written to
  `config.json`
- **AND** the user MUST be told that no credential is set and how to set it

#### Scenario: Presets are listable

- **WHEN** `tinyroute add --list` is run
- **THEN** each preset MUST be shown with its name, dialect, and base URL

#### Scenario: Dialect selection for dual-protocol providers

- **WHEN** a preset offers both `anthropic` and `openai` dialects and `--dialect openai` is given
- **THEN** the written entry MUST use the `openai` dialect and that dialect's base URL

#### Scenario: Editing a stale preset value requires no release

- **WHEN** a preset's base URL is out of date and the written entry fails
- **THEN** correcting the `base_url` in `config.json` MUST be sufficient to restore operation

#### Scenario: Presets are absent from the request path

- **WHEN** a provider exists in the preset table but not in `config.json`
- **THEN** routing to it MUST fail as an unknown provider

### Requirement: Validation reports actionable configuration errors

`tinyroute validate` SHALL check the configuration and report every problem found, including unknown
dialects, unknown providers referenced by chains, unset interpolation variables, malformed globs, and
routes whose `from` dialect does not match the dialect of a provider in their chain. Each error SHALL
name the offending route index or provider name. Exit status SHALL be non-zero when any error is
found.

#### Scenario: Dialect mismatch between surface and chain is detected

- **WHEN** a route declares `from: anthropic` and its chain includes a provider whose dialect is
  `openai`, and no translator is available for that pair
- **THEN** validation MUST fail identifying the route and stating that the pair requires translation
  unavailable in this build

#### Scenario: Chain references an undeclared provider

- **WHEN** a chain hop names a provider absent from the `providers` object
- **THEN** validation MUST fail naming the route index and the missing provider

#### Scenario: Clean configuration reports a summary

- **WHEN** the configuration has no errors
- **THEN** the command MUST report the provider and route counts and exit zero

### Requirement: Provider credentials are set without exposing them to the shell

`tinyroute auth set <provider>` SHALL read the credential from standard input and write it into the
provider's `api_key` field. The credential MUST NOT be accepted as a command-line argument, so that it
does not appear in shell history or process listings. `tinyroute auth list` SHALL display providers
with credentials masked to a short trailing fragment, and SHALL mark providers with no credential as
unset.

#### Scenario: Credential read from stdin

- **WHEN** `tinyroute auth set zai` is run and a value is supplied on standard input
- **THEN** the value MUST be stored as the `api_key` for `zai`
- **AND** MUST NOT appear in the command line

#### Scenario: Listing masks stored credentials

- **WHEN** `tinyroute auth list` is run with a credential stored for `anthropic` and none for `zai`
- **THEN** the `anthropic` credential MUST be shown masked
- **AND** `zai` MUST be shown as unset

### Requirement: Configured providers can be probed on demand

`tinyroute test` SHALL send a minimal probe request to verify that a chain is reachable and its
credentials are accepted. `tinyroute test --all` SHALL probe every configured provider. Results SHALL
report per-hop outcome, and failures SHALL distinguish an unreachable base URL from a rejected
credential from an unknown model.

#### Scenario: Probing a chain reports each hop

- **WHEN** `tinyroute test claude-sonnet-4-6` is run
- **THEN** each hop in the resolved chain MUST be reported with its outcome

#### Scenario: Rejected credential is distinguished from an unreachable host

- **WHEN** one provider returns an authentication failure and another cannot be reached
- **THEN** the output MUST distinguish the two conditions per provider
