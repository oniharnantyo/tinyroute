## MODIFIED Requirements

### Requirement: Auth subcommands accept an account label

`providers auth set`, `providers auth login`, and `providers auth import` SHALL accept
an `--account <name>` flag. When set, the credential SHALL be written under the
`provider/<name>` key (and, for `set`, into `Provider.Accounts[]` for that named
account) instead of the implicit `default`. All OAuth flow runners (device code,
PKCE, and provider-specific flows) SHALL honor the label; it SHALL NOT be dropped
between the command boundary and the credential store. The flag SHALL be honored
alongside the existing interactive/non-interactive control flags.

When the flag is unset:

- **WHEN** the provider has no existing connection, behavior SHALL be identical to
  before this change (the credential lands under the implicit `default`).
- **WHEN** the provider already has an existing connection, the account name SHALL
  be resolved through the provider-account-naming identity ladder (token-derived
  identity, then the first free slot), so an additional login never overwrites the
  stored credential.

#### Scenario: `auth set --account` writes into the named account

- **WHEN** `tinyroute providers auth set openai --account work` is run with a key
- **THEN** the key SHALL be stored as the credential of the `work` account
- **AND** SHALL NOT overwrite `Provider.APIKey`

#### Scenario: `auth login --account` keys the OAuth record by account

- **WHEN** `tinyroute providers auth login codex --account team2` completes the OAuth flow
- **THEN** the resulting tokens SHALL be stored under the `provider/team2` key
- **AND** any existing `default` record SHALL remain untouched

#### Scenario: Omitting `--account` preserves legacy behavior

- **WHEN** `providers auth set` / `login` / `import` is run without `--account`
  on a provider with no stored connection
- **THEN** the credential SHALL be written under the implicit `default` exactly as
  before this change

#### Scenario: Omitting `--account` on a connected provider creates an additional account

- **WHEN** `tinyroute providers auth login codex` completes the OAuth flow for a
  provider that already has a stored connection
- **THEN** the new tokens SHALL be stored under an account resolved through the
  identity ladder
- **AND** the pre-existing connection SHALL remain untouched

#### Scenario: The account label survives every flow runner

- **WHEN** `auth login --account <name>` runs a device-code, PKCE, or
  provider-specific flow
- **THEN** the stored record SHALL be keyed `provider/<name>`
