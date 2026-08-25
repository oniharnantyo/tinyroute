# management-dashboard Specification

## Purpose
TBD - created by archiving change serve-dashboard. Update Purpose after archive.
## Requirements
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

The dashboard SHALL render: a windowed overview of request volume, success rate, token usage, average latency, provider health with traffic, a request-volume chart, and top models by token usage; a providers list; a filterable, paginated request history; and API keys. The overview, providers, and history views SHALL source from read-only state (history aggregates and querier, topology watcher, health store, credential store) and MUST NOT mutate state. The API keys view is a management surface whose mutations are specified in "API keys are managed from the dashboard".

The dashboard SHALL NOT serve a routes view: no sidebar entry for routes SHALL
appear, and `GET /dashboard/routes` SHALL not be a registered route.

The overview SHALL NOT render a recent-failures list; failure investigation is served by the history view's outcome filter.

#### Scenario: overview reflects current state
- **WHEN** the user opens the overview with a window of 1h, 24h, 7d, or 30d (default 24h when absent or unsupported)
- **THEN** request count, success rate, token totals, and average latency are computed over only the records whose timestamps fall within that window, and each renders via the KPICard wrapper with compact number formatting for token counts

#### Scenario: window selection navigates with a query parameter
- **WHEN** the user selects a different window tab on the overview
- **THEN** the view reloads for the chosen window via a query parameter, so windowed URLs are shareable and work without client-side scripting

#### Scenario: overview renders a traffic chart
- **WHEN** the overview renders for a window
- **THEN** a request-volume chart renders via the templui chart component, bucketed server-side with bucket width derived from the window length, and empty buckets render as zero-height rather than being skipped

#### Scenario: provider panel combines health with window traffic
- **WHEN** the overview renders
- **THEN** each configured provider shows its cooldown status from the health store alongside its windowed request count and success rate, and each provider row links to that provider's detail view

#### Scenario: top models are ranked by windowed token usage
- **WHEN** the overview renders for a window containing records
- **THEN** a top-models table ranks models by combined input and output tokens within the window, composed from the templui table components

#### Scenario: overview auto-refreshes
- **WHEN** the overview remains open in a browser
- **THEN** it reloads periodically without user action so statistics stay current

#### Scenario: overview no longer lists failures
- **WHEN** the overview renders
- **THEN** no failures table is present, and failed requests remain inspectable through the history view's outcome filter

#### Scenario: history is filterable and paginated
- **WHEN** the user applies filters (provider/key/outcome/time) and paginates
- **THEN** matching history rows are returned through the existing history querier

#### Scenario: routes view is gone
- **WHEN** an authenticated user views the dashboard sidebar
- **THEN** no Routes entry appears
- **WHEN** `GET /dashboard/routes` is requested
- **THEN** the response status is `404`

### Requirement: API keys are managed from the dashboard

The dashboard SHALL provide an API keys management surface at `/dashboard/keys` comprising the key table and four actions. The table SHALL render one row per unrevoked key with: name, key identifier, masked secret with a reveal control, rate spec, expiry, and a binary status badge — **active** when the key is neither disabled nor expired, **inactive** otherwise. Revoked keys MUST NOT render.

The **Create** action SHALL open a dialog collecting a name (required), an expiry (optional; presets never / 7 days / 30 days, plus a custom absolute date), and a rate limit (optional; request count plus interval). On success the plaintext SHALL be shown exactly once with a copy control and the client environment snippet.

The **Edit** action SHALL open the same dialog pre-filled with the key's name, expiry, and rate. Editing SHALL NOT rotate the secret or make it editable; installed clients keep authenticating unchanged.

The **Revoke** action SHALL require an explicit confirmation dialog and SHALL be permanent — no enable action exists on any surface. On success the row disappears from the table.

The **Reveal** action SHALL unmask an active key's plaintext in place with a copy control. It MUST NOT be available for revoked keys.

All key mutations SHALL be `POST` requests on session-protected routes following the dashboard's redirect-with-error convention, and the plaintext of any key MUST NOT appear in a redirect URL, flash or query parameter, or log. When no unrevoked keys exist the view SHALL render an empty state with a create affordance. Icons SHALL be SVG via the templ `icon` component.

#### Scenario: the key table lists unrevoked keys with binary status

- **WHEN** the user opens `/dashboard/keys`
- **THEN** one row is shown per unrevoked key with name, identifier, masked secret, rate, expiry, and a status badge
- **AND** an expired-but-unrevoked key shows **inactive** while an enabled, unexpired key shows **active**

#### Scenario: revoked keys do not render

- **WHEN** the keys view renders and the keystore contains a disabled key
- **THEN** no row is shown for that key

#### Scenario: create shows the plaintext exactly once

- **WHEN** the user submits the Create dialog with a name and a 30-day expiry
- **THEN** the key is persisted with that expiry
- **AND** its plaintext is shown once with a copy control and the client environment snippet
- **AND** the plaintext does not appear in any redirect URL, flash parameter, or log

#### Scenario: edit changes constraints without rotating the credential

- **WHEN** the user edits a key's rate from 60/1m to 10/1m and saves
- **THEN** the stored rate is updated and the key's secret is unchanged

#### Scenario: revoke requires confirmation and is permanent

- **WHEN** the user chooses Revoke and confirms
- **THEN** the key is disabled, its row disappears, and no enable action is offered
- **AND** a request presenting that key's credential is rejected on the next request

#### Scenario: reveal unmasks an active key in place

- **WHEN** the user activates Reveal on an active key
- **THEN** the plaintext is displayed with a copy control
- **AND** activating Reveal is impossible for a revoked key because no row renders

#### Scenario: empty state offers creation

- **WHEN** no unrevoked keys exist
- **THEN** the view renders an empty state with a create affordance

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

### Requirement: Clients are managed from the dashboard

The dashboard SHALL provide a **Clients** management surface, reachable from a sidebar navigation item, that mirrors the `tinyroute clients` CLI. It SHALL comprise a clients list and a per-client live configuration editor.

The **clients list** at `/dashboard/clients` SHALL render one card per registered client sourced from the client registry (`clients.All()`). Each card SHALL display the client's name, its dialect, and a status badge derived from `Detect()`: **Connected** when the client is installed and pointed at tinyroute, **Not Configured** when installed but not pointed at tinyroute, and **Not Installed** when not present on the host. The entire card SHALL be a link to that client's detail view.

The **client detail view** at `/dashboard/clients/{id}` SHALL render a live configuration editor with: an in-page client switcher; a **Select endpoint** dropdown of the gateway's dialect-mapped endpoints (default = the endpoint derived from the client's dialect); a read-only **Current** field showing the endpoint the client currently points to; a masked **API key** field offering either minting a new key or selecting an existing key from a dropdown that SHALL list only active (not disabled) keys with a recoverable secret; one model picker per slot declared by the client's `ModelSlots()` (single slots as bounded pickers, multi-list slots as multi-selects, options from the models routable on that client's dialect, optional slots clearable); and a **Context window** field. It SHALL provide **Apply**, **Reset**, and **Manual config** actions.

The **Apply** action SHALL produce a structured **preview** (the client, the resolved endpoint, whether a key will be minted or a caller key used, the selected model slots, the target config path(s), and whether an existing file will be backed up), SHALL require **explicit confirmation**, and SHALL only then write configuration via the client adapter's `Apply()`. If the user does not confirm, no key SHALL be minted and no file SHALL be written. When a key is minted, its plaintext SHALL be revealed exactly once on the apply result with a copy control, and SHALL NOT appear in any redirect URL, flash parameter, or log. The **Reset** action SHALL remove only the fields tinyroute injected (after confirmation), preserving all other user settings. The **Manual config** action SHALL render the exact configuration snippet to paste manually.

Because clients write configuration to home-directory paths on the **gateway host**, the UI SHALL state that clients are configured on the gateway machine. All icons SHALL be SVG via the templ `icon` component (no emoji).

#### Scenario: the clients list shows a card per registered client

- **WHEN** the user opens `/dashboard/clients`
- **THEN** one card is shown for each registered client, with its name and dialect
- **AND** each card links to that client's detail view

#### Scenario: card status is derived from Detect

- **WHEN** the clients list renders
- **THEN** a client that is installed and pointed at tinyroute shows a Connected badge
- **AND** a client that is installed but not pointed shows a Not Configured badge
- **AND** a client that is not installed shows a Not Installed badge

#### Scenario: the detail view renders an in-page client switcher

- **WHEN** the user opens a client's detail view
- **THEN** a switcher is present that changes the viewed client without returning to the list

#### Scenario: the endpoint dropdown defaults to the derived endpoint

- **WHEN** the user opens the editor for a client whose dialect is anthropic on a gateway listening at 127.0.0.1:8787
- **THEN** the Select endpoint dropdown defaults to http://127.0.0.1:8787/anthropic
- **AND** the other gateway dialect endpoints are available in the dropdown

#### Scenario: the editor shows the client's current endpoint and masked key

- **WHEN** the editor renders for a configured client
- **THEN** the Current field shows the endpoint the client currently points to
- **AND** the API key field shows the configured key masked, never in plaintext

#### Scenario: model pickers are generated from the client's declared slots

- **WHEN** the editor renders for a client that declares model slots
- **THEN** one picker is rendered per declared slot, with options from the models routable on that client's dialect
- **AND** a client that declares no slots renders no model pickers

#### Scenario: Apply requires confirmation before writing

- **WHEN** the user submits the editor with Apply
- **THEN** a preview is shown and no file is written yet
- **AND** only after the user confirms is the configuration applied

#### Scenario: declining the preview writes nothing

- **WHEN** the user declines confirmation at the preview
- **THEN** no API key is minted, no config file is modified, and no backup is created

#### Scenario: a minted key is revealed exactly once

- **WHEN** the user chooses to mint a key and confirms Apply
- **THEN** a key is appended to the keystore and its plaintext is revealed once with a copy control
- **AND** the plaintext is not present in any redirect URL, flash parameter, or log

#### Scenario: the existing-key dropdown excludes revoked keys

- **WHEN** the editor's Provide Key dropdown is populated
- **THEN** disabled keys MUST NOT be offered
- **AND** keys without a recoverable secret MUST NOT be offered

#### Scenario: a caller-supplied key is written as-is

- **WHEN** the user chooses to reuse an existing token and confirms Apply
- **THEN** no key is minted or persisted and the supplied token is written into the client config

#### Scenario: Reset removes only injected fields after confirmation

- **WHEN** the user triggers Reset and confirms
- **THEN** only the fields tinyroute injected are removed and unrelated user settings are preserved
- **AND** declining confirmation leaves the config unchanged

#### Scenario: Manual config renders the snippet to paste

- **WHEN** the user triggers Manual config
- **THEN** the exact configuration snippet for that client is rendered for manual pasting

#### Scenario: host-local scope is made explicit

- **WHEN** the user views the clients list or an editor
- **THEN** the UI states that clients are configured on the gateway host machine

### Requirement: Icons are rendered as SVG, never emoji
The dashboard SHALL render every icon (status indicators, navigation, and actions) as SVG via the templ UI `icon` component (Lucide). It MUST NOT use emoji glyphs anywhere in the UI.

#### Scenario: no emoji glyphs in rendered output
- **WHEN** any dashboard page is rendered
- **THEN** all icons are SVG elements and no emoji characters are emitted

### Requirement: OAuth providers can be connected from the dashboard

For a provider whose preset declares OAuth capability, the dashboard SHALL offer a **Connect** action on the provider detail page that initiates an OAuth flow in the browser. The flow SHALL be driven by a reusable OAuth runner (PKCE authorization-code for standard presets, and RFC 8628 device-code for presets whose flow type is `device_code`), configured from the preset's OAuth constants (client id/secret, authorize/token/device endpoints, scopes, redirect URI, extra parameters). The dashboard SHALL persist the `code_verifier` and `state` server-side for PKCE flows and verify `state` at the callback. The account label (if the user supplied one, e.g. from a connect dialog `input`) SHALL be carried through the flow alongside `state` so the callback knows its target account. On success the resolved tokens SHALL be stored through the existing credential store (as an OAuth refresh credential) under an account resolved per the provider-account-naming identity ladder, the matching `Provider.Accounts[]` entry SHALL be upserted, and the dashboard SHALL reflect a masked connection. A subsequent connect SHALL never overwrite a different account's stored credential. Plaintext tokens SHALL NEVER be logged or emitted.

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

#### Scenario: a second connection creates an additional account

- **WHEN** the user connects a provider that already has a stored connection
- **THEN** the new tokens SHALL land under an account resolved through the identity ladder
- **AND** the pre-existing connection and its `Accounts[]` entry SHALL remain untouched
- **AND** completion SHALL surface a `toast` notification (variant: success) naming the account the connection was stored under, with no secret material in the copy

#### Scenario: the topology linkage is written with the credential

- **WHEN** an OAuth flow completes under an account name
- **THEN** the provider's `Accounts[]` SHALL contain a matching entry (`type: oauth_refresh`) after the save

### Requirement: Client detail header identifies the client by logo and status

The client detail view at `/dashboard/clients/{id}` SHALL render a header containing the
client's brand logo (looked up by client id from the embedded logo assets, with a monogram
fallback when no logo is embedded), the client's display name, and its status badge. The
header SHALL NOT render the raw client id/dialect metadata subtitle
(`ID: <id> • Dialect: <dialect>`).

#### Scenario: header shows the client's logo

- **WHEN** the user opens the detail view for a client with an embedded logo (e.g. `claude`)
- **THEN** the header renders that client's logo beside the client name and status badge

#### Scenario: header falls back to a monogram

- **WHEN** the user opens the detail view for a client with no embedded logo
- **THEN** the header renders a two-letter monogram derived from the client name

#### Scenario: header omits raw metadata

- **WHEN** the detail view renders
- **THEN** no `ID: <id> • Dialect: <dialect>` subtitle line appears in the header

### Requirement: Model picker options render as uniform rows in a modal dialog

Each model slot picker in the client detail editor SHALL open a modal dialog
(via the dialog component) when activated. The dialog body SHALL present two
tab panes built with the tabs component — **Models** and **Combos** — whenever
at least one combo is routable: the Models pane lists routable models grouped
by provider, and the Combos pane lists routable combos as a flat,
alphabetical, name-only list. When no combos are routable, the tab bar SHALL
be omitted and the dialog SHALL render the flat provider-grouped models list
directly. Each pane SHALL carry its own search input (via the input component)
that filters only that pane's option rows and group headers; a query in one
pane SHALL NOT affect the other. Within the dialog, every option row — the
`(None / Default)` clear option (for optional slots, rendered in both panes)
and each routable model or combo — SHALL render as the same styled row
component, and the currently selected option SHALL be visibly marked. The
dialog SHALL open with the Combos pane active when the slot's current value
is a combo id, and the Models pane active otherwise. Selecting an option
SHALL close the dialog and update the slot's value; the styling of option
rows SHALL NOT depend on client-side evaluation of an expression that embeds
static class names.

#### Scenario: picker opens as a modal dialog

- **WHEN** the user activates a slot's picker button and at least one combo is routable
- **THEN** a modal dialog opens, centered with a backdrop, with a Models pane listing
  the routable models grouped by provider and a Combos pane listing the routable
  combos as a flat alphabetical list

#### Scenario: tab bar is omitted when no combos are routable

- **WHEN** the picker dialog renders and no combos are routable
- **THEN** no Models/Combos tab bar renders, and the dialog shows the flat
  provider-grouped models list with its search input

#### Scenario: each pane filters with its own search input

- **WHEN** the user types a query into the Models pane's search input
- **THEN** only the Models pane's rows and group headers filter against the query,
  leaving the Combos pane's rows unaffected
- **AND** typing into the Combos pane's search input filters only the Combos pane

#### Scenario: group headers hide when no member rows match

- **WHEN** a search query in the Models pane matches no model of some provider group
- **THEN** that provider's group header is hidden along with its rows

#### Scenario: default tab follows the slot's current value

- **WHEN** the picker dialog opens for a slot whose current value is a combo id
- **THEN** the Combos pane is the active pane
- **WHEN** the picker dialog opens for any other slot value
- **THEN** the Models pane is the active pane

#### Scenario: option rows render uniformly and styled

- **WHEN** the picker dialog renders
- **THEN** every option row — including `(None / Default)`, each model, and each
  combo — displays as the same bordered row component with consistent spacing
- **AND** no option renders as unstyled inline content

#### Scenario: the selected option is visibly marked

- **WHEN** the picker dialog opens for a slot that has a value
- **THEN** the row for the currently selected model or combo carries a distinct
  selected-state accent and a check indicator

#### Scenario: selecting an option closes the dialog and updates the slot

- **WHEN** the user clicks an option row, in either pane
- **THEN** the dialog closes and the slot's field displays the chosen model or combo

#### Scenario: dialog is dismissible

- **WHEN** the picker dialog is open and the user presses Escape or clicks the backdrop
- **THEN** the dialog closes without changing the slot's value

### Requirement: Dashboard serves component behavior scripts

The dashboard SHALL serve the installed UI components' behavior scripts as static assets under
`/dashboard/assets/`, and the dashboard layout SHALL load the dialog component's script on
every page. The script SHALL be embedded in the binary, with no runtime dependency on
external hosts.

#### Scenario: dialog script is served

- **WHEN** a logged-in session requests `GET /dashboard/assets/dialog.js`
- **THEN** the response is the dialog component's JavaScript with a 200 status

#### Scenario: layout loads the script

- **WHEN** any dashboard page renders
- **THEN** the page includes a `defer` script tag for `/dashboard/assets/dialog.js`

### Requirement: History rows display their true HTTP status

The dashboard history list SHALL display, for each row, the HTTP status the client actually received, derived from the record: the status of the first successful (2xx) attempt when one exists, otherwise the last attempt's status, otherwise a mapping of the outcome category to the status the gateway returned (`no_route`→404, `auth_failed`→401, `rate_limited`→429, `body_too_large`→413, `chain_exhausted`→502, `mid_stream_failure`→502, `ok`→200). The rendered badge SHALL show the derived numeric status, not a hardcoded label. Records with malformed attempt data SHALL still render, using the outcome mapping.

#### Scenario: Successful request shows its real status

- **WHEN** a record's attempts contain a 2xx attempt with status 200
- **THEN** the row's status badge displays `200` with the success variant

#### Scenario: Failed chain shows the final failure status

- **WHEN** a record's attempts are `429` then `502` with no 2xx attempt
- **THEN** the row's status badge displays `502` with the destructive variant

#### Scenario: No-attempt failures map from outcome

- **WHEN** a record has outcome `no_route` and no attempts
- **THEN** the row's status badge displays `404`

#### Scenario: Malformed attempts JSON does not break rendering

- **WHEN** a record's attempts field fails to parse
- **THEN** the row still renders with a status derived from the outcome mapping

### Requirement: History is filterable by provider, date range, key, and session

The history list SHALL offer a provider filter as a dropdown whose options are sourced from live topology providers (plus an "All providers" default), a `from` date picker, a `to` date picker, a key filter, and a session filter. The `to` filter SHALL be inclusive of its entire day. All active filters SHALL be preserved in the URL across Load More activation.

#### Scenario: Provider options come from live state

- **WHEN** the user opens the provider filter
- **THEN** the dropdown lists every configured provider and no free-text entry is required

#### Scenario: Date range is inclusive of the end day

- **WHEN** the user filters with `to` set to a date on which requests exist
- **THEN** records from that entire day are included

#### Scenario: Filters survive Load More

- **WHEN** the user has active provider, date, key, or session filters and activates Load More
- **THEN** the additional rows continue to match every active filter

### Requirement: History pagination uses Load More

The history list SHALL paginate by growing the result window (Load More), not by cursor navigation. Each activation SHALL request the next increment of rows while preserving all filters. The page SHALL indicate the number of rows shown, and the result window SHALL be bounded by a server-side maximum.

#### Scenario: Load More appends within the same filter set

- **WHEN** 80 records match the filter and 50 are shown
- **THEN** activating Load More displays the next increment, ordered most-recent-first, with no duplicate or missing rows

#### Scenario: Load More is absent when the window covers all matches

- **WHEN** the number of matching records does not exceed the current window
- **THEN** no Load More control is rendered

### Requirement: Request detail page exposes captured bodies and attempt chain

The dashboard SHALL serve a per-request detail page at `/dashboard/history/{id}` behind dashboard authentication, showing the record's metadata (status, model, provider, latency, tokens), the full attempt chain (provider, model, status, latency per hop), and four body panes: the client request, the translated provider request, the raw provider response, and the final response delivered to the client. Each pane SHALL display its body size, be collapsed by default, pretty-print JSON bodies, and truncate bodies above a size cap with a visible notice. An unknown request ID SHALL render a not-found state with a link back to the history list.

#### Scenario: Detail page shows the four captured bodies

- **WHEN** the user opens the detail page for a proxied request
- **THEN** the client request, translated provider request, raw provider response, and final response are each present as separate collapsible panes

#### Scenario: Oversized bodies are truncated with a notice

- **WHEN** a stored body exceeds the size cap
- **THEN** the pane renders a truncation notice and a bounded prefix of the body

#### Scenario: Unknown request ID

- **WHEN** the user navigates to `/dashboard/history/{id}` for an ID that does not exist
- **THEN** a not-found state renders with a link back to the history list

#### Scenario: Attempt chain renders per hop

- **WHEN** a record contains multiple attempts
- **THEN** each hop's provider, model, status, and latency are displayed in order

### Requirement: API keys are added as accounts from the dashboard

The provider detail page's add-API-key form (a `dialog` with a `input` for the
secret and an optional `input` for an account label) SHALL append a
`Provider.Accounts[]` entry (`{name, type: static, api_key}`) resolved through the
provider-account-naming ladder (explicit label, else first free slot). It SHALL
NOT overwrite the scalar `provider.api_key` or any existing account's key. A
successful add SHALL show a `toast` notification (variant: success) naming the
account; a rejected add (empty secret, invalid label) SHALL show a `toast`
(variant: destructive) describing the constraint. No plaintext key SHALL appear in
any toast copy beyond the existing masked display.

#### Scenario: first key creates a named account

- **WHEN** the user submits an API key with label `work` on a provider with no accounts
- **THEN** an account `work` (`type: static`) SHALL hold the key in `Accounts[]`
- **AND** a success `toast` SHALL name the account

#### Scenario: second key adds another account instead of replacing

- **WHEN** the user submits another API key without a label on a provider that already has an account
- **THEN** a new slot account SHALL be appended holding the new key
- **AND** the existing account's key SHALL be unchanged

#### Scenario: empty secret is rejected

- **WHEN** the add-key dialog is submitted with an empty secret
- **THEN** a destructive `toast` SHALL state that the key is required
- **AND** no topology change SHALL occur

### Requirement: Connections can be reconnected and renamed from the dashboard

Each masked connection row on the provider detail page SHALL offer **Reconnect**
and **Rename** actions (via the row's `dropdown` menu). Reconnect SHALL start the
provider's OAuth flow with the row's account name as the explicit label, so
completed flows rotate that account's tokens in place. Rename SHALL re-key the
stored credential (`provider/<old>` → `provider/<new>`) and rewrite the matching
`Accounts[].Name` atomically in one gesture, after a `dialog` confirmation. A
rename to an existing name SHALL be rejected with a destructive `toast`. Success
SHALL show a success `toast` naming the account; no toast copy SHALL contain token
material. Rename SHALL additionally rewrite every combo member pinned to the old
account name (`provider@old:model` → `provider@new:model`) in the same write, so
no combo is left pinning an account the provider no longer declares.

#### Scenario: reconnect rotates a specific account in place

- **WHEN** the user picks Reconnect on connection `jane@example.com` and completes the flow
- **THEN** only that account's tokens SHALL be replaced
- **AND** a success `toast` SHALL name the account

#### Scenario: rename re-keys credential and topology together

- **WHEN** the user renames account `account-2` to `team-pool` and confirms the `dialog`
- **THEN** the credential store SHALL contain `provider/team-pool` and no `provider/account-2`
- **AND** the `Accounts[]` entry SHALL be renamed to `team-pool`
- **AND** a success `toast` SHALL name the new account

#### Scenario: rename to an existing name is rejected

- **WHEN** the user renames an account to a name already present on the provider
- **THEN** a destructive `toast` SHALL report the collision
- **AND** neither the credential store nor the topology SHALL change

#### Scenario: rename rewrites pinned combo members in the same write

- **WHEN** the user renames account `work` to `team` and a combo holds member
  `glm@work:glm-4.7`
- **THEN** that member SHALL become `glm@team:glm-4.7` in the same write as the
  `Accounts[]` rename
- **AND** a success `toast` SHALL name the account

### Requirement: Combos section in dashboard navigation

The dashboard SHALL provide a Combos section reachable from the sidebar
navigation, positioned between Providers and History, using the Lucide layers
icon. The section SHALL list all configured combos.

#### Scenario: Combos nav entry is reachable

- **WHEN** an authenticated user views the dashboard
- **THEN** the sidebar SHALL contain a Combos entry between Providers and
  History, with the layers icon, linking to the combos list

### Requirement: Combo list with ordered members

The combos page SHALL render one card per combo (via the card component)
showing the combo name at the base text size (one step larger than body
text), a mode badge (via the badge component), and members as
position-numbered chips conveying member order. Member chips SHALL display
the `provider:model` form — any `@account` pin SHALL be stripped from the
card display while stored member strings remain verbatim (apparent
duplicates from same-model-different-account members SHALL render as-is).
Each card SHALL show at most 3 member chips; when a combo has more, an
overflow marker `+N more…` (N = total − 3) SHALL follow the list, carrying
the hidden members in its hover title. Disabled combos SHALL render
visually muted so their state is legible beyond the toggle position. Each
card SHALL offer enable/disable (see the toggle requirement), edit, and
delete actions. When no combos exist, the page SHALL show an empty state
(via the empty component with the layers icon) explaining what a combo is,
with a single call-to-action to create one.

#### Scenario: List renders numbered member order

- **WHEN** the combos page is viewed and a combo has members
  `["anthropic:claude-sonnet-4.5", "glm:glm-4.7", "openai:gpt-5.2"]`
- **THEN** the card SHALL display the members numbered 1, 2, 3 in that order

#### Scenario: Card title renders larger than body text

- **WHEN** a combo card is rendered
- **THEN** the combo name SHALL use the base text size, larger than the
  card's member and metadata text

#### Scenario: Member list caps at three with an overflow marker

- **WHEN** a combo has 5 members
- **THEN** the card SHALL render 3 member chips followed by `+2 more…`
- **AND** the overflow marker's hover title SHALL list the 2 hidden members

#### Scenario: Account pins are stripped from card display

- **WHEN** a combo member is stored as `glm@work:glm-4.7`
- **THEN** the card SHALL display it as `glm:glm-4.7`
- **AND** editing the combo SHALL still open with `glm@work:glm-4.7`
  verbatim

#### Scenario: Disabled combos render muted

- **WHEN** a combo has `disabled: true`
- **THEN** its card SHALL render with reduced emphasis relative to enabled
  cards

#### Scenario: Empty state with no combos

- **WHEN** no combos are configured
- **THEN** the page SHALL render an empty-state component describing combos
  with a create call-to-action, not a blank page

### Requirement: Combo cards can be toggled enabled or disabled

Each combo card SHALL carry an enable/disable switch (via the switch
component, `role=switch`) in the card footer, left of the edit action,
reflecting the combo's current state. Activating the switch SHALL submit a
form POST to `/dashboard/combos/toggle` carrying the combo name — no
client-side fetch or JavaScript beyond the shared component bundle. The
handler SHALL flip the combo's disabled flag, persist configuration, and
redirect back to the combos page with feedback. Because the gateway
watches the configuration file, the change SHALL take effect on the live
gateway without a restart.

#### Scenario: Toggle flips state and persists

- **WHEN** the user activates the switch on enabled combo `coding-priority`
- **THEN** the dashboard SHALL persist `disabled: true` for that combo
  and re-render the card with the switch off and muted styling

#### Scenario: Toggle takes effect without restart

- **WHEN** a combo is disabled from the dashboard while the gateway is
  running
- **THEN** the next request for that combo name SHALL fail with the
  explicit disabled error, with no gateway restart

#### Scenario: Toggle is a plain form POST

- **WHEN** the switch is rendered
- **THEN** it SHALL sit inside a form POSTing to `/dashboard/combos/toggle`
  with the combo name as a hidden field, covered by the dashboard's
  existing cross-origin protections

### Requirement: Combo creation wizard hosted in a dialog

Creating a combo SHALL happen inside a dialog (via the dialog component)
opened from the combos page's "New Combo" button, without navigating away
from the list. The wizard SHALL present the same five steps as the CLI wizard
(name, members in priority order, mode, optional capabilities, review) one
step at a time, with back/continue navigation, and SHALL NOT write
configuration until the final create action. Editing an existing combo SHALL
reuse the same dialog flow pre-filled with current values.

#### Scenario: Wizard opens in a dialog over the list

- **WHEN** the user activates "New Combo"
- **THEN** a dialog SHALL open over the combos list showing the name step
- **AND** the list SHALL remain visible behind it

#### Scenario: Step navigation within the dialog

- **WHEN** the user completes a step and continues, or goes back
- **THEN** the dialog SHALL advance or return one step, preserving previously
  entered values

#### Scenario: Nothing written until final create

- **WHEN** the user cancels or dismisses the dialog at any step
- **THEN** no configuration change SHALL occur

#### Scenario: Edit reuses the wizard pre-filled

- **WHEN** the user activates edit on a combo card
- **THEN** the same dialog wizard SHALL open pre-filled with that combo's
  current name, members in their configured order, mode, and capabilities

#### Scenario: Pinned members round-trip through edit

- **WHEN** the user edits a combo whose members include `glm@work:glm-4.7`
- **THEN** the wizard SHALL open with that member listed verbatim (pin
  intact)
- **AND** saving without changes SHALL preserve the member string exactly

### Requirement: Member selection conveys priority order

The members step SHALL add members one at a time through two dropdowns —
**Model** and **Connection** — appending each addition to a numbered list
whose position conveys member order. Each row SHALL offer move-up,
move-down, and remove controls (icon buttons via the button component).
At least one member SHALL be required to continue.

The Model dropdown SHALL list each whitelisted model exactly once per
provider (native `<select>` with `<optgroup>` provider grouping), in
`provider:model` form — the model list SHALL NOT be duplicated per account.
The Connection dropdown SHALL render visible but disabled with a
"Select a model first" placeholder until a model is chosen. When a model is
selected, the Connection dropdown SHALL be enabled and scoped to that
model's provider only: it SHALL offer **Any connection** (no account pin;
the provider's selection strategy applies) plus each account that provider
declares when it declares two or more (accounts in sorted order), defaulting
to the first account. When the selected model's provider declares fewer than
two accounts, the dropdown SHALL remain disabled on Any connection (the pin
would be semantically inert). The per-provider account list SHALL ship with
the page as a data island and the scoping SHALL be applied client-side on
model change — a presentational option-list sync only; all wizard state and
validation remain server-driven. Activating Add SHALL compose the member
server-side — selected model with Any connection yields `provider:model`;
selected model with account `work` yields `provider@work:model` — and SHALL
reject the addition with an inline error when the posted account belongs to
a provider other than the selected model's provider (a guard for crafted
posts; the scoped dropdown cannot produce this pairing) or when the composed
member is already in the list. After a successful add, both dropdowns SHALL
reset to their defaults.

#### Scenario: Added member appends as next priority

- **WHEN** two members are listed and a third is added
- **THEN** it SHALL appear as position 3 and the member order SHALL reflect
  the list top-to-bottom

#### Scenario: Reordering changes priority

- **WHEN** the user moves a member up one position
- **THEN** its position SHALL swap with the member above it

#### Scenario: A single member is enough to continue

- **WHEN** the members list holds one member and the user continues
- **THEN** the wizard SHALL advance to the mode step
- **AND** continuing with an empty list SHALL re-render the step with an
  inline error

#### Scenario: Model dropdown lists each model once per provider

- **WHEN** the members step renders for provider `glm` with accounts `work`
  and `personal` whitelisting `glm-4.7`
- **THEN** the Model dropdown SHALL contain `glm-4.7` exactly once under the
  `glm` group
- **AND** no `provider@account` options SHALL appear in the Model dropdown

#### Scenario: Connection dropdown is disabled until a model is selected

- **WHEN** the members step renders with no model chosen
- **THEN** the Connection dropdown SHALL be visible but disabled with a
  "Select a model first" placeholder

#### Scenario: Connection dropdown scopes to the selected model's provider

- **WHEN** the user selects Model `glm:glm-4.7` where `glm` declares
  accounts `work` and `personal`
- **THEN** the Connection dropdown SHALL be enabled with a `glm` group
  offering `personal` and `work`, plus Any connection
- **AND** it SHALL default to `glm`'s first account (`personal`, sorted
  first) with Any connection available as an explicit choice
- **AND** it SHALL NOT offer accounts of any other provider

#### Scenario: Single-account provider keeps the dropdown on Any

- **WHEN** the user selects a model of a provider declaring fewer than two
  accounts
- **THEN** the Connection dropdown SHALL remain disabled on Any connection

#### Scenario: Model plus connection composes a pinned member

- **WHEN** the user selects Model `glm:glm-4.7` and Connection `work` and
  activates Add
- **THEN** the member `glm@work:glm-4.7` SHALL be appended verbatim
- **AND** both dropdowns SHALL reset to their defaults

#### Scenario: Any connection composes an unpinned member

- **WHEN** the user selects Model `glm:glm-4.7` with Connection left at Any
  connection and activates Add
- **THEN** the member `glm:glm-4.7` SHALL be appended

#### Scenario: Mismatched connection is rejected

- **WHEN** a POST composes Model `openai:gpt-5.2` with Connection `work`
  (an account of `glm`)
- **THEN** the step SHALL re-render with an inline error naming the mismatch
- **AND** no member SHALL be appended

#### Scenario: Duplicate composed member is rejected

- **WHEN** Add is activated for a composed member identical to one already
  in the list
- **THEN** the step SHALL re-render with an inline error
- **AND** the member SHALL NOT be duplicated

### Requirement: Combo deletion requires confirmation

Deleting a combo SHALL require confirmation via a confirmation dialog (the
dialog component with destructive action styling, consistent with key
revocation) naming the combo. Only the confirm action SHALL remove the combo
from configuration.

#### Scenario: Delete confirms before removing

- **WHEN** the user activates delete on a combo card and confirms the
  confirmation dialog
- **THEN** the combo SHALL be removed and the list SHALL re-render without it

#### Scenario: Cancelled delete keeps the combo

- **WHEN** the user dismisses the delete confirmation
- **THEN** the combo SHALL remain configured and listed

### Requirement: Combo mutations notify via toast

Successful combo creation, edit, and deletion SHALL surface feedback through a
success toast (via the toast component) naming the combo and the action. The
toast SHALL contain no secrets — only the combo name, action, and member
count.

#### Scenario: Create success toast

- **WHEN** a combo is created through the wizard
- **THEN** a success toast SHALL appear with copy naming the created combo
- **AND** the toast SHALL NOT contain credentials or secrets

#### Scenario: Edit and delete toasts

- **WHEN** a combo is edited or deleted
- **THEN** a success toast SHALL appear naming the combo and the action
  performed

### Requirement: Connection removal keeps account-pinned combo members consistent

Removing a connection from the provider detail page SHALL downgrade every
combo member pinned to that account (`provider@account:model`) to its
unpinned form (`provider:model`), preserving the provider and model. When the
unpinned form is already a member of the same combo, the downgraded entry
SHALL be dropped instead of duplicated. A connection removal SHALL NOT
remove any combo — downgrade preserves the provider and model of every
member, so each combo keeps at least one member. The confirmation dialog SHALL
warn when combos will be affected, and the completion toast SHALL name every
combo that was modified. A combo SHALL never survive a connection removal
referencing an account the provider no longer declares.

#### Scenario: Pinned member downgrades on disconnect

- **WHEN** the user disconnects account `work` on provider `glm` and a combo
  holds member `glm@work:glm-4.7`
- **THEN** the member SHALL become `glm:glm-4.7` in the same write
- **AND** the toast SHALL name the combo that was modified

#### Scenario: Downgrade to an existing member drops the pin instead

- **WHEN** a combo holds both `glm:glm-4.7` and `glm@work:glm-4.7` and
  account `work` is disconnected
- **THEN** the pinned entry SHALL be dropped, leaving the member list
  unchanged in meaning and free of duplicates

#### Scenario: All-pinned combo survives downgrade with one member

- **WHEN** a combo's members all pin the disconnected account and downgrade
  would deduplicate them into one
- **THEN** the combo SHALL survive with that single unpinned member
- **AND** the toast SHALL name the combo that was modified

