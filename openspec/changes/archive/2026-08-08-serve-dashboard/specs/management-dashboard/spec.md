## ADDED Requirements

### Requirement: Dashboard is served by the serve command
The `serve` command SHALL serve a web dashboard on the existing gateway listener, mounted under `/dashboard/*`. The dashboard SHALL be enabled by default, and the user MUST be able to disable it with a `--no-dashboard` flag. Requests to `/dashboard/*` MUST NOT be recorded into proxy request history.

#### Scenario: serve starts the dashboard by default
- **WHEN** the user runs `serve` without `--no-dashboard`
- **THEN** the dashboard is reachable at `/dashboard/*` on the gateway listener

#### Scenario: --no-dashboard disables the dashboard
- **WHEN** the user runs `serve --no-dashboard`
- **THEN** no `/dashboard/*` routes are served and only proxy endpoints remain

#### Scenario: dashboard traffic is not recorded as proxy history
- **WHEN** a request hits a `/dashboard/*` route
- **THEN** it is not written to the request history store

### Requirement: Browser opens automatically in interactive environments
The dashboard SHALL open in the user's default browser when `serve` starts and an interactive display is available. It MUST NOT attempt to open a browser in non-interactive or headless environments, and in those cases it SHALL log the dashboard URL instead.

#### Scenario: interactive start opens the browser
- **WHEN** `serve` starts with the dashboard enabled and an interactive display is present
- **THEN** the user's browser opens to the dashboard URL

#### Scenario: headless start does not open a browser
- **WHEN** `serve` starts with no interactive display
- **THEN** no browser is launched and the dashboard URL is logged

### Requirement: Dashboard access requires a password
The dashboard SHALL require a password to access any route under `/dashboard/*`. The default password SHALL be `123456` when no dashboard auth file exists. The password MUST be stored bcrypt-hashed in `~/.tinyroute/dashboard.json` at file mode `0600`, written atomically. A successful login SHALL establish a `SameSite=Strict` session cookie.

#### Scenario: first run seeds the default password
- **WHEN** `serve` starts and `~/.tinyroute/dashboard.json` does not exist
- **THEN** the file is created with a bcrypt hash of `123456` at mode `0600`

#### Scenario: correct password grants access
- **WHEN** the user submits the correct password
- **THEN** a `SameSite=Strict` session cookie is set and access is granted

#### Scenario: incorrect password is rejected
- **WHEN** the user submits an incorrect password
- **THEN** access is denied and no session cookie is set

### Requirement: Mutating actions are protected from cross-origin abuse
Every dashboard request that mutates state (POST) SHALL be rejected unless its `Host` header is a local loopback host (`localhost` or `127.0.0.1`) and it carries a valid session cookie. Login attempts SHALL be rate-limited via the existing rate limiter, keyed on the client address.

#### Scenario: mutating POST from a non-loopback Host is rejected
- **WHEN** a POST arrives whose `Host` header is not a loopback host
- **THEN** the request is rejected

#### Scenario: repeated failed logins are throttled
- **WHEN** a client exceeds the login rate limit
- **THEN** further login attempts from that client are rejected for the cooldown period

### Requirement: Password can be changed from the dashboard
The dashboard SHALL provide a settings screen to change the password. Submitting it MUST update the bcrypt hash in `~/.tinyroute/dashboard.json` atomically and take effect for subsequent logins without restarting `serve`.

#### Scenario: changing the password persists and applies immediately
- **WHEN** the user sets a new password in settings
- **THEN** the stored hash is updated atomically, the new password works on the next login, and the old password no longer does

### Requirement: Observe views render live gateway state
The dashboard SHALL render: an overview of request volume, success rate, token usage, and provider health; a providers list; routes; a filterable, paginated request history; and API keys. These views SHALL source from existing read APIs (history querier, topology watcher, health store, credential store) and MUST NOT mutate state.

#### Scenario: overview reflects current state
- **WHEN** the user opens the overview
- **THEN** request volume, success rate, token totals, provider health, and recent failures are displayed from live state

#### Scenario: history is filterable and paginated
- **WHEN** the user applies filters (provider/key/outcome/time) and paginates
- **THEN** matching history rows are returned through the existing history querier

### Requirement: Providers are listed as compact cards and managed on a detail view
The providers list SHALL render each provider as a compact card showing only its logo (or monogram fallback), display name, and connection count. Clicking a card SHALL open a provider detail view at `/dashboard/providers/{name}` that hosts all provider information and management: a header (dialect, base URL, health, connection count), a Connections section (masked, from `Provider.Accounts` enriched with `credential.Store.ListMasked()`), a Models section (lean model cards plus an add-model control sourced from the catalog/live model APIs), and read-only advanced settings. Adding providers (via a preset card picker with logos) and all mutations SHALL go through the existing topology mutators (`ParseRawTopology` → `WriteTopology`) so `${VAR}` references are preserved and writes are atomic at mode `0600`. Secret fields (API keys, tokens) MUST be write-only in the UI and masked in any JSON the dashboard emits.

The **connection count** is the number of the provider's accounts (`len(Accounts)`); if there are none and a direct `APIKey` is set, the count is 1; otherwise 0.

#### Scenario: providers list shows compact cards
- **WHEN** the user opens the providers list
- **THEN** each provider is shown as a card with only its logo, display name, and connection count

#### Scenario: clicking a card opens the detail view
- **WHEN** the user clicks a provider card
- **THEN** the detail view at `/dashboard/providers/{name}` renders the provider's header, connections, models, and advanced settings

#### Scenario: connections are shown masked
- **WHEN** the detail view renders the Connections section
- **THEN** each account/token is displayed masked (never plaintext), sourced from `Provider.Accounts` and `credential.Store.ListMasked()`

#### Scenario: adding a provider via a preset card
- **WHEN** the user picks a preset card and saves
- **THEN** the provider is added to the topology via the mutators with `${VAR}` references preserved

#### Scenario: existing secrets are never revealed
- **WHEN** the user manages a provider that has an API key configured
- **THEN** the key field is write-only (never displayed) and masked in any JSON response

### Requirement: Model whitelists are managed as lean cards
For each whitelisted model, the dashboard SHALL render a card showing only: the tinyroute model name (`provider:model`), the provider model name, a copy control for the tinyroute model name, a Test action, and a remove control. The Test action SHALL reuse the existing model probe and report status and latency. Adding and removing models SHALL update the provider's whitelist through the topology mutators.

#### Scenario: model card shows both names and actions
- **WHEN** a whitelisted model is displayed
- **THEN** the card shows the tinyroute model name, the provider model name, a copy control, a Test action, and a remove control

#### Scenario: test runs the probe
- **WHEN** the user triggers Test on a model
- **THEN** the probe is sent to the provider and status with latency is reported

#### Scenario: copying the tinyroute model name
- **WHEN** the user triggers copy on a model card
- **THEN** the `provider:model` string is placed on the clipboard

### Requirement: Icons are rendered as SVG, never emoji
The dashboard SHALL render every icon (status indicators, navigation, and actions) as SVG via the templ UI `icon` component (Lucide). It MUST NOT use emoji glyphs anywhere in the UI.

#### Scenario: no emoji glyphs in rendered output
- **WHEN** any dashboard page is rendered
- **THEN** all icons are SVG elements and no emoji characters are emitted
