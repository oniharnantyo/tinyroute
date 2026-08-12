## ADDED Requirements

### Requirement: Connection and whitelist actions lazily materialize available presets

A provider SHALL be considered **available** when it is present in the topology OR is a known preset. Any dashboard mutation that targets an available preset not yet in the topology — saving a static API key, completing an OAuth flow, or adding a model to the whitelist — SHALL first write a topology entry materialized from that preset's defaults, then apply the mutation to that entry. This materialization SHALL be idempotent (a no-op when the entry already exists) and SHALL go through the existing topology mutators (`ParseRawTopology` → `WriteTopology`) so `${VAR}` references are preserved and writes are atomic at mode `0600`. There SHALL be no separate prerequisite "Configure" step before these actions succeed.

#### Scenario: saving a static API key on an available preset materializes it

- **WHEN** the user saves an API key for a preset that is not yet in the topology
- **THEN** a topology entry is written from the preset defaults
- **AND** the API key is stored on that entry
- **AND** the provider becomes routable on topology reload

#### Scenario: whitelisting a model on an available preset materializes it

- **WHEN** the user adds a model to the whitelist of a preset that is not yet in the topology
- **THEN** a topology entry is written from the preset defaults
- **AND** the model is appended to that entry's whitelist
- **AND** no "Provider not found" error is returned

#### Scenario: completing an OAuth flow on an available preset materializes it

- **WHEN** an OAuth callback resolves tokens for a preset that is not yet in the topology
- **THEN** a topology entry is written from the preset defaults before the credential is recorded
- **AND** the resolved tokens are stored through the credential store
- **AND** the provider becomes routable on topology reload

#### Scenario: materialization is idempotent for an already-configured provider

- **WHEN** a mutation targets a provider already present in the topology
- **THEN** no duplicate topology entry is created
- **AND** the mutation is applied to the existing entry

#### Scenario: a non-existent provider is still rejected

- **WHEN** a mutation targets a name that is neither in the topology nor a known preset
- **THEN** the request is rejected with a clear error
- **AND** no topology entry is written

### Requirement: Provider status on the detail page is connection-derived

The provider detail header SHALL display a status derived from connection state rather than a binary "configured" flag. The status SHALL be one of: **Connected** (in topology with a resolvable credential), **Awaiting credentials** (in topology without a credential), **Cooldown** (in the auth-failure cooldown window), or **Not connected** (an available preset with no topology entry and no credential). The detail page SHALL NOT render the amber "Provider Not Configured" banner or a standalone Configure action.

#### Scenario: an available preset shows Not connected

- **WHEN** the user opens the detail page for a preset with no topology entry and no credential
- **THEN** the header shows a "Not connected" status
- **AND** no Configure banner or Configure action is rendered

#### Scenario: an activated provider without a credential shows Awaiting credentials

- **WHEN** the user opens the detail page for a provider in the topology that has no credential
- **THEN** the header shows an "Awaiting credentials" status

#### Scenario: a connected provider shows Connected

- **WHEN** the user opens the detail page for a provider in the topology with a resolvable credential
- **THEN** the header shows a "Connected" status (or "Cooldown" when in the auth-failure window)

### Requirement: Deleting a preset-backed provider reverts it to a clean available preset

Deleting a provider whose name matches a known preset SHALL remove its topology entry and any stored credentials, then leave the preset available in the providers list as an unconnected card. Deleting a custom (non-preset) provider SHALL remove it entirely. In both cases the underlying topology mutation SHALL go through the existing mutators with atomic `0600` writes.

#### Scenario: deleting a preset-backed provider resets it

- **WHEN** the user deletes a provider that is backed by a known preset
- **THEN** its topology entry and stored credentials are removed
- **AND** the preset reappears in the providers list as an available, unconnected card

#### Scenario: deleting a custom provider removes it

- **WHEN** the user deletes a provider that is not backed by any preset
- **THEN** its topology entry is removed
- **AND** it no longer appears in the providers list

## MODIFIED Requirements

### Requirement: Providers are listed as compact cards and managed on a detail view

The providers list SHALL render every known provider — both configured providers and all available presets — as compact cards, grouped into sections in this fixed order: **Free Tier** (every preset whose `tier` is `free` or `freemium`), then **OAuth** (remaining OAuth-capable presets), then **API Key** (all remaining presets). Each preset SHALL appear in exactly one section; a tiered preset SHALL be pulled into the Free Tier section regardless of its auth type. Each card SHALL show the logo (or monogram fallback), a Title-Cased display name, the dialect, and CLI-style auth/tier badges. The **entire card SHALL be a link** to the provider detail view at `/dashboard/providers/{name}`; the card SHALL NOT display a base URL, a health flag, or separate Manage/Configure buttons. A client-side search control SHALL filter cards across all sections. The detail view hosts provider management: a header (Title-Cased display name, dialect, base URL, health, connection count), a Connections section (masked, from `Provider.Accounts` enriched with `credential.Store.ListMasked()`), and a Models section. A provider enters the topology lazily — the first connection or whitelist action on an available preset writes its entry; there is no separate Configure step. All mutations SHALL go through the existing topology mutators (`ParseRawTopology` → `WriteTopology`) so `${VAR}` references are preserved and writes are atomic at mode `0600`. Secret fields (API keys, tokens) MUST be write-only in the UI and masked in any JSON the dashboard emits. The read-only Endpoint Settings section SHALL NOT be rendered.

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

#### Scenario: connecting to an available preset materializes it

- **WHEN** the user performs a connection or whitelist action on an available preset from the detail page (saves an API key, completes OAuth, or whitelists a model)
- **THEN** the provider is added to the topology via the mutators with `${VAR}` references preserved
- **AND** the mutation is applied to that entry

### Requirement: Model whitelists are managed as lean cards

The provider detail page SHALL render a single **Models** section listing every model the provider offers: all catalog models (sourced from `config.LoadOrRefreshCatalog`) plus any whitelisted model not present in the catalog. Each model SHALL be rendered as a card annotated with whether it is whitelisted. A whitelisted model SHALL show the tinyroute model name (`provider:model`), a copy control, a Test action (reusing the existing model probe), and a remove control. A non-whitelisted model SHALL show an add control that appends the model to the provider's whitelist via the existing add-model endpoint. Adding and removing models SHALL update the provider's whitelist through the topology mutators. The Models section SHALL provide a client-side filter and a "show more" control so large catalogs remain usable. The Models section SHALL render for every available provider (configured or a preset not yet in the topology); it SHALL NOT be gated on configuration state, and no Configure prompt or banner SHALL be shown in its place.

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

#### Scenario: an available preset renders the Models section

- **WHEN** the user opens the detail view for a preset that is not yet in the topology
- **THEN** the Models section is rendered from the catalog
- **AND** no Configure prompt or banner is shown

#### Scenario: whitelisting a model on an available preset materializes the provider

- **WHEN** the user adds a model from the Models section of a preset not yet in the topology
- **THEN** the preset is first materialized into the topology and the model is appended to its whitelist
