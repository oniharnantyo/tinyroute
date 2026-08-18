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

The dashboard SHALL render: an overview of request volume, success rate, token usage, and provider health; a providers list; routes; a filterable, paginated request history; and API keys. The overview, providers, routes, and history views SHALL source from existing read APIs (history querier, topology watcher, health store, credential store) and MUST NOT mutate state. The API keys view is a management surface whose mutations are specified in "API keys are managed from the dashboard".

#### Scenario: overview reflects current state
- **WHEN** the user opens the overview
- **THEN** request volume, success rate, token totals, provider health, and recent failures are displayed from live state

#### Scenario: history is filterable and paginated
- **WHEN** the user applies filters (provider/key/outcome/time) and paginates
- **THEN** matching history rows are returned through the existing history querier

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

Each model slot picker in the client detail editor SHALL open a modal dialog when activated.
Within the dialog, every option row — the `(None / Default)` clear option (for optional slots)
and each routable model — SHALL render as the same styled row component, and the currently
selected option SHALL be visibly marked. Selecting an option SHALL close the dialog and update
the slot's value; the styling of option rows SHALL NOT depend on client-side evaluation of an
expression that embeds static class names.

#### Scenario: picker opens as a modal dialog

- **WHEN** the user activates a slot's picker button
- **THEN** a modal dialog opens, centered with a backdrop, listing the routable models grouped
  by provider

#### Scenario: option rows render uniformly and styled

- **WHEN** the picker dialog renders
- **THEN** every option row — including `(None / Default)` and each model — displays as the
  same bordered row component with consistent spacing
- **AND** no option renders as unstyled inline content

#### Scenario: the selected option is visibly marked

- **WHEN** the picker dialog opens for a slot that has a value
- **THEN** the row for the currently selected model carries a distinct selected-state accent
  and a check indicator

#### Scenario: selecting an option closes the dialog and updates the slot

- **WHEN** the user clicks an option row
- **THEN** the dialog closes and the slot's field displays the chosen model

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

