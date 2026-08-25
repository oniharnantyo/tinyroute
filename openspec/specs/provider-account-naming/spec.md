# provider-account-naming Specification

## Purpose

Governs how every credential write path assigns an account name to a provider
connection, so that connecting an additional OAuth account or API key never
overwrites an existing one and reconnecting the same identity rotates it in place.

## Requirements

### Requirement: Account names resolve through a deterministic identity ladder

Every credential write path (OAuth connect, API-key add, credential import) SHALL
resolve the target account name through the following stages, in order, using the
first stage that yields a usable name:

1. an explicit account label supplied by the user;
2. an identity claim derived from the token response — the `email` claim of an
   `id_token`, or the `email`/`sub` claim of a JWT-formatted access token;
3. the first free auto-generated slot (`account-2`, `account-3`, …).

Derivation from token claims SHALL NOT require signature verification: claims are
used only to produce a display label, and SHALL be sanitized before use. When no
stage yields a name before stage 3, stage 3 always succeeds.

#### Scenario: Explicit label wins over token claims

- **WHEN** a connect or add supplies an explicit account label
- **THEN** the credential SHALL be stored under that label
- **AND** any identity claim present in the token response SHALL be ignored for naming

#### Scenario: Token-derived identity names the account when no label is given

- **WHEN** an OAuth flow completes without an explicit label and the token response
  contains an `id_token` with an `email` claim
- **THEN** the credential SHALL be stored under an account named from that email

#### Scenario: Opaque tokens fall back to an auto-generated slot

- **WHEN** an OAuth flow completes without an explicit label and the token response
  carries no identity material (e.g. an opaque device-flow token)
- **THEN** the credential SHALL be stored under the first free `account-N` name

### Requirement: Existing accounts update in place; derived collisions never replace

WHEN the resolved account name already exists for the provider:

- **WHEN** the name was supplied explicitly (or targets a connection the user chose
  to reconnect)
- **THEN** the existing account's credential SHALL be updated in place
  (refresh-token rotation) and no other account SHALL be modified
- **WHEN** the name was derived from the identity ladder (stages 2–3 re-run because
  the derived name is taken)
- **THEN** the write path SHALL move to the next free name instead of overwriting

No credential write path SHALL silently replace the credential of a different
account.

#### Scenario: Reconnecting the same identity rotates in place

- **WHEN** an OAuth flow for an identity already stored as account `jane@example.com`
  completes again
- **THEN** that account's tokens SHALL be replaced
- **AND** no other account of the provider SHALL change

#### Scenario: Derived name collision takes the next free slot

- **WHEN** a derived name equals an existing account of the same provider
- **THEN** the write SHALL land under the next free slot name
- **AND** the pre-existing account SHALL remain untouched

### Requirement: Account names are validated before use as store keys

Account names SHALL match a bounded charset (letters, digits, and `-`, `_`, `.`,
`@`), SHALL NOT contain `/` or whitespace, and SHALL be at most 64 characters.
An explicit name failing validation SHALL be rejected with an error naming the
constraint. A derived name failing validation SHALL be sanitized; if it cannot be
salvaged, resolution SHALL fall through to the next ladder stage.

#### Scenario: Slash in an explicit account name is rejected

- **WHEN** a user supplies `--account foo/bar`
- **THEN** the command SHALL fail with an error naming the account-name constraint
- **AND** nothing SHALL be written

#### Scenario: Derived claim is sanitized before use

- **WHEN** a token claim yields a value containing characters outside the charset
- **THEN** the stored account name SHALL be a sanitized form of that value