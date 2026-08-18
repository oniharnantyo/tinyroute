# Management Dashboard Delta

## ADDED Requirements

### Requirement: API keys are managed from the dashboard

The dashboard SHALL provide an API keys management surface at
`/dashboard/keys` comprising the key table and four actions. The table SHALL
render one row per unrevoked key with: name, key identifier, masked secret
with a reveal control, rate spec, expiry, and a binary status badge —
**active** when the key is neither disabled nor expired, **inactive**
otherwise. Revoked keys MUST NOT render.

The **Create** action SHALL open a dialog collecting a name (required), an
expiry (optional; presets never / 7 days / 30 days, plus a custom absolute
date), and a rate limit (optional; request count plus interval). On success
the plaintext SHALL be shown exactly once with a copy control and the client
environment snippet.

The **Edit** action SHALL open the same dialog pre-filled with the key's
name, expiry, and rate. Editing SHALL NOT rotate the secret or make it
editable; installed clients keep authenticating unchanged.

The **Revoke** action SHALL require an explicit confirmation dialog and SHALL
be permanent — no enable action exists on any surface. On success the row
disappears from the table.

The **Reveal** action SHALL unmask an active key's plaintext in place with a
copy control. It MUST NOT be available for revoked keys.

All key mutations SHALL be `POST` requests on session-protected routes
following the dashboard's redirect-with-error convention, and the plaintext of
any key MUST NOT appear in a redirect URL, flash or query parameter, or log.
When no unrevoked keys exist the view SHALL render an empty state with a
create affordance. Icons SHALL be SVG via the templ `icon` component.

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

## MODIFIED Requirements

### Requirement: Observe views render live gateway state

The dashboard SHALL render: an overview of request volume, success rate, token usage, and provider health; a providers list; routes; a filterable, paginated request history; and API keys. The overview, providers, routes, and history views SHALL source from existing read APIs (history querier, topology watcher, health store, credential store) and MUST NOT mutate state. The API keys view is a management surface whose mutations are specified in "API keys are managed from the dashboard".

#### Scenario: overview reflects current state
- **WHEN** the user opens the overview
- **THEN** request volume, success rate, token totals, provider health, and recent failures are displayed from live state

#### Scenario: history is filterable and paginated
- **WHEN** the user applies filters (provider/key/outcome/time) and paginates
- **THEN** matching history rows are returned through the existing history querier

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
