# provider-credentials Specification

## ADDED Requirements

### Requirement: Auth subcommands accept an account label

`providers auth set`, `providers auth login`, and `providers auth import` SHALL accept
an `--account <name>` flag. When set, the credential SHALL be written under the
`provider/<name>` key (and, for `set`, into `Provider.Accounts[]` for that named
account) instead of the implicit `default`. When unset, behavior SHALL be identical to
today (single-key providers and `"provider/default"` records are unchanged). The flag
SHALL be honored alongside the existing interactive/non-interactive control flags.

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
- **THEN** the credential SHALL be written exactly as before this change
- **AND** existing single-credential providers SHALL behave identically
