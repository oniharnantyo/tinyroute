## ADDED Requirements

### Requirement: Provider Model Whitelist Configuration
The system SHALL allow models to be configured as a whitelist array (`Models []string`) directly on a `Provider` struct in the topology configuration.

#### Scenario: Configuration Parsing
- **WHEN** the `config.json` is parsed with a provider containing a `models` array
- **THEN** the system successfully maps the whitelist string array to the `Provider` struct in memory.

### Requirement: Interactive Model Addition
The system SHALL provide an interactive command `tinyroute provider model add <provider>` to fetch, select, and whitelist models.

#### Scenario: Successful Interactive Addition
- **WHEN** the user runs `tinyroute provider model add openai`
- **THEN** the system fetches available models, displays a multi-select prompt, and saves the selected models to the provider's whitelist in `config.json`.

### Requirement: Model Catalog Caching
The system SHALL cache fallback catalog data (e.g. from `models.dev/api.json`) locally at `~/.tinyroute/cache/api.json` with a 12-hour TTL and SHA256 checksum verification to ensure integrity.

#### Scenario: Cache Hit Within TTL
- **WHEN** a catalog fetch is requested and the local cache file is younger than 12 hours and checksum is valid
- **THEN** the system reads and parses the models directly from the local cache instead of making a network request.

#### Scenario: Cache Miss or Expired
- **WHEN** a catalog fetch is requested and the cache is expired or missing
- **THEN** the system fetches the remote catalog, atomitcally updates the cache via a `.tmp` file, updates the checksum, and parses the models.

### Requirement: List Whitelisted Models
The system SHALL provide a command `tinyroute provider model list <provider>` to display the currently whitelisted models.

#### Scenario: Listing Models
- **WHEN** the user runs `tinyroute provider model list openai`
- **THEN** the system outputs the contents of the `openai` provider's model whitelist.

### Requirement: Remove Whitelisted Models
The system SHALL provide a command `tinyroute provider model remove <provider> <model>` to delete a model from the whitelist.

#### Scenario: Removing a Model
- **WHEN** the user runs `tinyroute provider model remove openai gpt-4o`
- **THEN** the system removes `gpt-4o` from the `openai` provider's whitelist in `config.json`.

### Requirement: Test Whitelisted Model
The system SHALL provide a command `tinyroute provider model test <provider> <model>` to send a health probe.

#### Scenario: Testing a Model
- **WHEN** the user runs `tinyroute provider model test openai gpt-4o`
- **THEN** the system issues a minimal probe payload to the provider for `gpt-4o` and reports the HTTP status code and latency.
