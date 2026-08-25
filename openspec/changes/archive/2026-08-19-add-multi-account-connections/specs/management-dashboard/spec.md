## MODIFIED Requirements

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

## ADDED Requirements

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
material.

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
