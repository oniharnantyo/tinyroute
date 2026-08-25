# Tasks: Add Multi-Account Connections

Reference: specs in `specs/` (what), design.md (how). Follow repo TDD rules —
write the failing test first for each behavior, keep coverage ≥80% per package
touched. Run `gofmt -w .` before committing.

## 1. Account-name resolver (`internal/credential`)

- [x] 1.1 Write `naming_test.go` covering `ValidateAccountName`: valid names
       (letters, digits, `-_.@`), rejects `/`, whitespace, empty, >64 chars — RED
- [x] 1.2 Implement `ValidateAccountName` in `naming.go` — GREEN
- [x] 1.3 Write tests for `SanitizeDerivedName`: email → sanitized label, claims
       with hostile characters, unsalvageable input → `""` — RED, then implement
- [x] 1.4 Write tests for `ResolveAccount` ladder per
       `provider-account-naming` spec: explicit wins; identity hint used when no
       label; explicit collision → same name (in place); derived collision → next
       free slot starting at `account-2`; no existing names + no inputs →
       `default` — RED
- [x] 1.5 Implement `ResolveAccount` — GREEN, all resolver tests pass

## 2. Identity capture in OAuth flows

- [x] 2.1 Add `id_token` to the token-response structs in `internal/oauth/oauth.go`
       (PKCE exchange + device poll); capture raw access token
- [x] 2.2 Implement JWT-payload decode helper (`email`/`sub` from unverified
       payload; only when token has three dot-separated segments) with tests using
       fixed base64url fixtures
- [x] 2.3 Add `IdentityHint string` (with `json:"-"`) to `OAuthRecord`; populate
       it in `oauth.ExchangePKCE` and `oauth.PollDeviceFlow`; test that marshalled
       credentials.json never contains the hint
- [x] 2.4 Populate `IdentityHint` in the four CLI runners in
       `internal/cli/auth.go` (device, PKCE, qoder, trae)

## 3. CLI account threading

- [x] 3.1 Thread an account-name parameter through `runDeviceCodeFlow`,
       `runPKCEFlow`, `runQoderFlow`, `runTraeFlow`; each sets `rec.Account`
       before `store.Save` (table-driven tests per runner where testable)
- [x] 3.2 In `cmdAuthLoginWithAccount`: when no label and the store already holds
       any record for the provider, resolve via `ResolveAccount` after the flow
       (union of store keys + topology `Accounts[].Name` as `existing`); fresh
       provider keeps `default` — tests for both branches
- [x] 3.3 Fix `cmdAccountAdd` (`--type oauth_refresh`): pass account name as
       explicit label to the delegated flow, propagate the error, write topology
       only after flow success — test: failed flow leaves topology unchanged
- [x] 3.4 Verify `auth login --account <name>` end-to-end stores under
       `provider/<name>` and leaves `default` untouched (spec scenario)

## 4. Topology account upsert (`internal/config`)

- [x] 4.1 TDD `Provider.UpsertAccount`: appends when new, replaces by name,
       no-op shape preserved (sort/identity stable) — RED then implement
- [x] 4.2 Add a helper that performs the save gesture used by write paths:
       `store.Save` → `ensureMaterialized` → `UpsertAccount` → `WriteTopology`,
       store-first with errors naming the failed half; wire into dashboard deps

## 5. Dashboard — connect dialog + account-aware OAuth

- [x] 5.1 Extend `OAuthStateSession` with `Account string`; accept an optional
       label form value in `handleOAuthStart`, store it in the session (PKCE) and
       redirect params (device)
- [x] 5.2 In `handleOAuthCallback` and `handleOAuthDevicePoll`: resolve the
       account via `ResolveAccount` (explicit label from session, else
       `IdentityHint`, else slot; `existing` = store keys + topology accounts),
       set on the record, save, then upsert `Accounts[]` per the D5 gesture
- [x] 5.3 Connect affordance becomes a `dialog` (secret-free: optional label
       `input`) posting to `/oauth/start`; success `toast` names the account,
       per `management-dashboard` delta spec
- [x] 5.4 Handler tests: second connect on a connected provider creates a new
       account and leaves `default` untouched; completion upserts a matching
       `Accounts[]` entry (spec scenarios)

## 6. Dashboard — API keys as accounts

- [x] 6.1 Rework `handleProviderCredential`: resolve name (label input, else
       first free slot), append `Account{Name, Type: "static", APIKey}` via the
       D5 gesture; never write the scalar `api_key`
- [x] 6.2 Move the add-key form into a `dialog` with secret + optional label
       `input`s; success/destructive `toast` behavior per spec
- [x] 6.3 Handler tests: first key creates a named account; second key appends
       (elder unchanged); empty secret rejected with no topology write; scalar
       `api_key` from legacy configs remains intact after an add

## 7. Dashboard — reconnect + rename

- [x] 7.1 Add Reconnect to each connection row's `dropdown`: starts the flow with
       the row's account as explicit label (reuses 5.1/5.2 plumbing); test
       asserts in-place rotation, other accounts untouched
- [x] 7.2 Add `handleProviderAccountRename`: validate new name, reject
       collision, re-key store record, rewrite `Accounts[].Name`, store-first
       ordering per D7 — tests for happy path, collision, and unknown account
- [x] 7.3 Rename UI: `dropdown` action → `dialog` confirmation with `input`;
       `toast` feedback per spec (no token material in copy)
- [x] 7.4 Verify per-connection Disconnect (existing) still deletes both the
       store record and the `Accounts[]` entry

## 8. Hardening + verification

- [x] 8.1 Edge tests: `/` in explicit label rejected end-to-end (CLI + dashboard);
       unsalvageable derived claim falls through to slot; sparse slot numbering
       tolerated
- [x] 8.2 Security pass: no plaintext tokens/keys in toasts, redirects, or logs;
       account names never echo secrets; credentials.json still written 0600
       atomic (tmp+rename)
- [x] 8.3 `gofmt -w .`, `go build ./...`, `go test ./...` green; new-code coverage
       ≥80% (naming.go 100%, jwt.go ≥80%); package baselines unchanged
- [x] 8.4 `openspec validate add-multi-account-connections` passes; walk each
       spec scenario against the implementation and record results
