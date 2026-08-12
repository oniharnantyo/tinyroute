## MODIFIED Requirements

### Requirement: Providers are listed as compact cards and managed on a detail view

The providers list SHALL render every known provider — both configured providers and all available presets — as compact cards, grouped into sections in this fixed order: **Free Tier** (every preset whose `tier` is `free` or `freemium`), then **OAuth** (remaining OAuth-capable presets), then **API Key** (all remaining presets). Each preset SHALL appear in exactly one section; a tiered preset SHALL be pulled into the Free Tier section regardless of its auth type. Each card SHALL show the logo (or monogram fallback), a Title-Cased display name, the dialect, and CLI-style auth/tier badges. The **entire card SHALL be a link** to the provider detail view at `/dashboard/providers/{name}`; the card SHALL NOT display a base URL, a health flag, or separate Manage/Configure buttons. A client-side search control SHALL filter cards across all sections. The detail view hosts provider management: a header (Title-Cased display name, dialect, base URL, health, connection count), a Connections section (masked, from `Provider.Accounts` enriched with `credential.Store.ListMasked()`), and a Models section. Adding providers and all mutations SHALL go through the existing topology mutators (`ParseRawTopology` → `WriteTopology`) so `${VAR}` references are preserved and writes are atomic at mode `0600`. Secret fields (API keys, tokens) MUST be write-only in the UI and masked in any JSON the dashboard emits. The read-only Endpoint Settings section SHALL NOT be rendered.

The **connection count** is the number of the provider's accounts (`len(Accounts)`); if there are none and a direct `APIKey` is set, the count is 1; otherwise 0.

#### Scenario: providers list shows compact cards

- **WHEN** the user opens the providers list
- **THEN** each provider is shown as a compact card with its logo, Title-Cased display name, dialect, and auth/tier badges
- **AND** configured providers additionally show their connection count

#### Scenario: all providers shown grouped by tier then auth

- **WHEN** the user opens the providers list
- **THEN** both configured providers and unconfigured presets are shown as cards
- **AND** the cards are grouped into Free Tier, OAuth, and API Key sections in that order

#### Scenario: a tiered preset is pulled into the Free Tier section

- **WHEN** a preset declares `tier` of `free` or `freemium`
- **THEN** it appears in the Free Tier section
- **AND** it does not also appear in the OAuth or API Key section

#### Scenario: clicking a card opens the detail view

- **WHEN** the user clicks a provider card
- **THEN** the detail view at `/dashboard/providers/{name}` opens
- **AND** the entire card is the link (no separate Manage/Configure button, base URL, or health flag is shown)

#### Scenario: card titles are Title-Cased

- **WHEN** a card is rendered for a provider without a preset `DisplayName`
- **THEN** its title is the Title-Cased name (e.g. `opencode-zen` → `Opencode Zen`, `anthropic` → `Anthropic`)

#### Scenario: search filters cards across all sections

- **WHEN** the user types into the providers search control
- **THEN** cards matching the query are shown and non-matching cards are hidden across every section

#### Scenario: connections are shown masked

- **WHEN** the detail view renders the Connections section
- **THEN** each account/token is displayed masked (never plaintext), sourced from `Provider.Accounts` and `credential.Store.ListMasked()`

#### Scenario: endpoint settings are not rendered

- **WHEN** the detail view renders
- **THEN** no Endpoint Settings / advanced-settings section is present

#### Scenario: existing secrets are never revealed

- **WHEN** the user manages a provider that has an API key configured
- **THEN** the key field is write-only (never displayed) and masked in any JSON response

#### Scenario: adding a provider via a preset card

- **WHEN** the user configures a preset (e.g. via the detail page Configure action) and saves
- **THEN** the provider is added to the topology via the mutators with `${VAR}` references preserved

### Requirement: Model whitelists are managed as lean cards

The provider detail page SHALL render a single **Models** section listing every model the provider offers: all catalog models (sourced from `config.LoadOrRefreshCatalog`) plus any whitelisted model not present in the catalog. Each model SHALL be rendered as a card annotated with whether it is whitelisted. A whitelisted model SHALL show the tinyroute model name (`provider:model`), a copy control, a Test action (reusing the existing model probe), and a remove control. A non-whitelisted model SHALL show an add control that appends the model to the provider's whitelist via the existing add-model endpoint. Adding and removing models SHALL update the provider's whitelist through the topology mutators. The Models section SHALL provide a client-side filter and a "show more" control so large catalogs remain usable. When the provider is **not configured**, the detail page SHALL NOT render the Models section; instead it SHALL render a message encouraging the user to configure the provider, with a Configure action that adds the provider via the preset add flow.

#### Scenario: model card shows both names and actions

- **WHEN** a whitelisted model is displayed
- **THEN** the card shows the model name, a copy control, a Test action, and a remove control

#### Scenario: a catalog model is added from the Models section

- **WHEN** the user clicks the add control on a non-whitelisted model card
- **THEN** the model is appended to the provider's whitelist via the existing add-model endpoint
- **AND** the page refreshes to show the model as active

#### Scenario: a custom whitelisted model not in the catalog is shown

- **WHEN** a whitelisted model is absent from the catalog
- **THEN** it still appears in the Models section as an active card

#### Scenario: test runs the probe

- **WHEN** the user triggers Test on a whitelisted model
- **THEN** the probe is sent to the provider and status with latency is reported

#### Scenario: copying the tinyroute model name

- **WHEN** the user triggers copy on a model card
- **THEN** the `provider:model` string is placed on the clipboard

#### Scenario: a large catalog stays usable

- **WHEN** a provider's catalog contains a large number of models
- **THEN** the Models section offers a client-side filter and a show-more control

#### Scenario: an unconfigured provider shows a configure prompt instead of models

- **WHEN** the user opens the detail view for a preset that is not yet configured
- **THEN** a message encourages the user to configure the provider
- **AND** a Configure action adds the provider via the preset add flow
- **AND** no Models section is rendered

## ADDED Requirements

### Requirement: OAuth providers can be connected from the dashboard

For a provider whose preset declares OAuth capability, the dashboard SHALL offer a **Connect** action on the provider detail page that initiates an OAuth flow in the browser. The flow SHALL be driven by a reusable OAuth runner (PKCE authorization-code for standard presets, and RFC 8628 device-code for presets whose flow type is `device_code`), configured from the preset's OAuth constants (client id/secret, authorize/token/device endpoints, scopes, redirect URI, extra parameters). The dashboard SHALL persist the `code_verifier` and `state` server-side for PKCE flows and verify `state` at the callback. On success the resolved tokens SHALL be stored through the existing credential store (as an OAuth refresh credential) and the dashboard SHALL reflect a masked connection. Plaintext tokens SHALL NEVER be logged or emitted.

#### Scenario: a PKCE provider is connected in-browser

- **WHEN** the user clicks Connect on an OAuth-capable PKCE provider
- **THEN** the browser is redirected to the provider's authorize endpoint
- **AND** after consent the callback exchanges the code for tokens and stores a masked OAuth connection

#### Scenario: a device-code provider is connected

- **WHEN** the user clicks Connect on a device-flow provider
- **THEN** a device code and verification URI are presented
- **AND** polling completes the flow and stores a masked OAuth connection once the user authorizes

#### Scenario: connection status is shown

- **WHEN** an OAuth-capable provider already has a stored OAuth connection
- **THEN** the detail page shows the masked connection status (e.g. connected / expiry) alongside the Connect action

#### Scenario: plaintext tokens are never exposed

- **WHEN** an OAuth flow completes or a connection is displayed
- **THEN** access/refresh tokens are not written to logs or rendered in the UI
- **AND** only a masked digest is shown
