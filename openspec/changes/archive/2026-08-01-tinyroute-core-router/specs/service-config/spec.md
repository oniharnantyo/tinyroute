## ADDED Requirements

### Requirement: Deployment settings are loaded from a dotenv file at startup

The service SHALL read deployment settings from a `.env` file once during process startup. Settings
SHALL be immutable for the lifetime of the process. The parser SHALL support `KEY=value` lines,
`#` comment lines, blank lines, optionally quoted values, and a tolerated leading `export `. The
parser MUST be implemented against the standard library only.

#### Scenario: Settings are read at startup

- **WHEN** `tinyroute serve` starts and `.env` contains `TINYROUTE_LISTEN=127.0.0.1:9000`
- **THEN** the HTTP listener MUST bind to `127.0.0.1:9000`

#### Scenario: Comments, blanks, quotes and export prefixes are handled

- **WHEN** `.env` contains a `#` comment line, a blank line, `export TINYROUTE_CAPTURE=full`, and
  `TINYROUTE_COOLDOWN_429="90s"`
- **THEN** the comment and blank line MUST be ignored, capture mode MUST be `full`, and the 429
  cooldown default MUST be 90 seconds with quotes stripped

#### Scenario: Changing the file has no effect until restart

- **WHEN** `.env` is edited while the daemon is running
- **THEN** the running process MUST continue using the values read at startup
- **AND** the new values MUST take effect only after a restart

### Requirement: The real process environment takes precedence over the dotenv file

An environment variable already present in the process environment MUST NOT be overridden by a value
in the `.env` file. The `.env` file SHALL act as a source of defaults only, so that containers,
systemd units, and external secret managers can inject settings directly.

#### Scenario: Injected variable wins

- **WHEN** the process is started with `TINYROUTE_LISTEN=0.0.0.0:8080` in its environment and `.env`
  contains `TINYROUTE_LISTEN=127.0.0.1:8787`
- **THEN** the listener MUST bind to `0.0.0.0:8080`

#### Scenario: Dotenv fills only unset variables

- **WHEN** the process environment does not define `TINYROUTE_CAPTURE` and `.env` defines it
- **THEN** the value from `.env` MUST be used

### Requirement: Dotenv file discovery follows a fixed order without merging

The service SHALL resolve the dotenv file from, in order: an explicit `--env-file` flag, then `./.env`
in the working directory, then `~/.tinyroute/.env`. The first existing file SHALL be used and other
candidates MUST be ignored. Multiple files MUST NOT be merged. A missing dotenv file MUST NOT be an
error when all required settings resolve from defaults or the environment.

#### Scenario: Explicit flag overrides discovery

- **WHEN** `tinyroute serve --env-file /etc/tinyroute/prod.env` is run and `./.env` also exists
- **THEN** only `/etc/tinyroute/prod.env` MUST be read

#### Scenario: Working directory takes precedence over home

- **WHEN** both `./.env` and `~/.tinyroute/.env` exist and no flag is given
- **THEN** only `./.env` MUST be read
- **AND** settings present solely in `~/.tinyroute/.env` MUST NOT be applied

#### Scenario: No dotenv file present

- **WHEN** no dotenv file exists at any candidate location
- **THEN** the service MUST start using defaults and environment values without reporting an error

### Requirement: Recognized deployment settings and their defaults

The service SHALL recognize the settings below and apply the stated defaults when a setting is
absent. Unrecognized `TINYROUTE_`-prefixed variables SHALL produce a warning and be ignored.

| Setting | Default |
|---|---|
| `TINYROUTE_LISTEN` | `127.0.0.1:8787` |
| `TINYROUTE_CONFIG` | `~/.tinyroute/config.json` |
| `TINYROUTE_KEYS` | `~/.tinyroute/keys.json` |
| `TINYROUTE_STATE` | `~/.tinyroute/state.json` |
| `TINYROUTE_HISTORY` | `~/.tinyroute/requests.jsonl` |
| `TINYROUTE_BLOBS` | `~/.tinyroute/blobs` |
| `TINYROUTE_CAPTURE` | `full` |
| `TINYROUTE_INJECT_USAGE` | `true` |
| `TINYROUTE_COOLDOWN_429` | `60s` |
| `TINYROUTE_COOLDOWN_5XX` | `10s` |
| `TINYROUTE_TRUST_PROXY` | `false` |
| `TINYROUTE_TLS_CERT` | empty (TLS disabled) |
| `TINYROUTE_TLS_KEY` | empty (TLS disabled) |

#### Scenario: Defaults applied when unset

- **WHEN** the dotenv file and environment define none of the above
- **THEN** the listener MUST bind `127.0.0.1:8787` and paths MUST resolve under `~/.tinyroute/`

#### Scenario: Unknown setting is reported and ignored

- **WHEN** `.env` contains `TINYROUTE_FEATURE_X=1`
- **THEN** a warning naming `TINYROUTE_FEATURE_X` MUST be emitted
- **AND** startup MUST continue

#### Scenario: Malformed duration is rejected at startup

- **WHEN** `.env` contains `TINYROUTE_COOLDOWN_429=soon`
- **THEN** startup MUST fail with an error naming the setting and the invalid value

### Requirement: TLS is static-certificate only and disabled by default

When both `TINYROUTE_TLS_CERT` and `TINYROUTE_TLS_KEY` are set, the listener SHALL serve HTTPS using
those files. When neither is set, the listener SHALL serve plain HTTP. Certificate acquisition and
renewal MUST NOT be implemented.

#### Scenario: Both cert and key provided

- **WHEN** both TLS settings point to readable files
- **THEN** the listener MUST serve HTTPS

#### Scenario: Only one of the pair provided

- **WHEN** `TINYROUTE_TLS_CERT` is set and `TINYROUTE_TLS_KEY` is empty
- **THEN** startup MUST fail with an error stating both are required together

### Requirement: Global settings are overridable by per-provider configuration

Settings that also exist per provider or per route SHALL be treated as global defaults, and a value
declared in `config.json` for a specific provider or route MUST take precedence. Precedence SHALL be
one-directional: per-provider overrides global, and never the reverse.

#### Scenario: Provider overrides the global cooldown

- **WHEN** `TINYROUTE_COOLDOWN_429=60s` and provider `zai` declares a 429 cooldown of `120s`
- **THEN** a 429 from `zai` MUST apply a 120 second cooldown
- **AND** a 429 from a provider without an override MUST apply 60 seconds

### Requirement: The dotenv file holds no secrets

Provider credentials and inbound API keys MUST NOT be read from the dotenv file or from
`TINYROUTE_`-prefixed variables. Provider credentials SHALL live in `config.json`; inbound keys SHALL
live in `keys.json`. This keeps the dotenv file free of secret material.

#### Scenario: No credential settings are recognized

- **WHEN** `.env` defines a variable intended to carry a provider credential for use as a
  `TINYROUTE_` setting
- **THEN** it MUST NOT be recognized as a deployment setting
- **AND** the provider credential MUST be resolved from `config.json` instead

#### Scenario: Interpolation references remain available

- **WHEN** `config.json` contains `"api_key": "${ANTHROPIC_API_KEY}"` and that variable is present in
  the process environment
- **THEN** the reference MUST resolve at config load time
