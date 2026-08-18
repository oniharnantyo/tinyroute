# Tasks

## 1. Rename `agent` → `clients` (behavior-preserving)

- [x] 1.1 Move package `internal/agent` → `internal/clients` (`package clients`): rename dir and update `package agent` → `package clients` in every file (registry, types, 13 adapters, helpers).
- [x] 1.2 Rename the interface `Agent` → `Client`; update `Register/Get/All` callers and all adapter receiver types (`*claudeAdapter` etc. remain; their methods now satisfy `Client`).
- [x] 1.3 Update all internal references to `agent.Status`, `agent.ApplyInput`, `agent.Result`, `agent.ModelSlot`, `agent.SlotSingle/SlotMulti` → `clients.*`.
- [x] 1.4 Rename `internal/cli/agent.go` → `internal/cli/clients.go`; rename command `tinyroute agent` → `tinyroute clients` (Name: "clients"); rename `cmdAgent*` → `cmdClient*`; update command registration in the CLI tree.
- [x] 1.5 Update all `*_test.go` references (agent_test.go, adapters_test.go, format_test.go, fileutil_test.go, claude_test.go, codex_test.go, agent_integration_test.go) to the new package/command names.
- [x] 1.6 Verify: `gofmt -w .`, `go build ./...`, `go test ./...` green; `tinyroute clients status` output identical to the former `tinyroute agent status`.

## 2. Extract TTY-free `clients.Installer`

- [x] 2.1 Create `internal/clients/installer.go` (interface + types only, per coding-style contract separation): `InstallRequest`, `Plan`, `InstallResult`, `KeyStrategy` constants, and the `Installer` signature (`Plan`, `Apply`, `MintKey`).
- [x] 2.2 Implement `Installer.Plan(req)`: resolve adapter by id, derive base URL from listen + dialect (move `dialectBaseURL` here), resolve key strategy, map requested slots onto `ModelSlots()`, compute target paths + backup flag — without writing. Return error for unknown id.
- [x] 2.3 Implement `Installer.Apply(plan)`: mint key when strategy=mint (move mint logic + `MintKey`), then call `adapter.Apply(ApplyInput{...})`; return `InstallResult{Files, Key, Backup}`.
- [x] 2.4 Implement `Installer.MintKey(req)`: `tr_live_` key scoped `<dialect>:*`, name default `client-<id>`, persisted via the keystore.
- [x] 2.5 Write `internal/clients/installer_test.go` (TDD): Plan returns preview without writes; Apply mints+writes on a mint plan; reuse writes caller token as-is, no mint; base-URL derivation per dialect (anthropic/openai/gemini) + `--base-url` override; slot mapping for single + multi; unknown id rejected at Plan; no-write-on-decline is structural (Plan/Apply split).
- [x] 2.6 Verify: `go test ./internal/clients/...` green.

## 3. CLI delegation + parity

- [x] 3.1 Rewrite `cmdClientInstall` to gather inputs via `interactive.*` and delegate to `Installer` (Plan → render text preview → `interactive.Confirm` → Apply). Preserve flags (`--api-key`, `--name`, `--model`, `--base-url`, `--interactive`/`--no-interactive`/`--force`).
- [x] 3.2 Rewrite `cmdClientStatus` and `cmdClientUninstall` over the registry (`clients.All/Get`, `Detect`, `Reset`); uninstall keeps its confirm guard.
- [x] 3.3 Update `internal/cli/clients_test.go` (and integration test) to assert the CLI routes through `Installer` and that observable output/exit behavior is unchanged vs. the pre-rename baseline (capture golden output where helpful).
- [x] 3.4 Verify: `go test ./internal/cli/...` green; manual `tinyroute clients install --force` dry path matches former behavior.

## 4. Serve-path wiring

- [x] 4.1 Add a blank import of the client adapters on the serve path (`internal/cli/serve.go`, alongside the dialect blank-imports) so `clients.All()` is populated in the dashboard/serve process.
- [x] 4.2 Add a regression test asserting `len(clients.All()) > 0` when the serve entry imports the adapters.

## 5. Extend `Detect()` for the live editor

- [x] 5.1 Extend `clients.Status` with `CurrentBaseURL`, `MaskedKey`, and `SlotValues map[string]string` (current values read from the client's own config file).
- [x] 5.2 Update each adapter's `Detect()` to populate the new fields by parsing its config file (e.g. claude reads `env.ANTHROPIC_BASE_URL` / `env.ANTHROPIC_AUTH_TOKEN` / the tier env vars); mask the key in `Detect()` (view-ready), never return plaintext.
- [x] 5.3 Add/extend adapter tests: configured client returns correct current URL + masked key + slot values; unconfigured client returns zero values; plaintext never appears in `Status`.

## 6. Dashboard: clients list

- [x] 6.1 Add data types in `internal/dashboard` (e.g. `view_clients.go`): `ClientsPageData`, `ClientCard{ID, Name, Dialect, Status}`; `Status` derived from `Detect()`: `connected` / `not_configured` / `not_installed`.
- [x] 6.2 Add `GET /dashboard/clients` route → `handleClientsView`; render `Layout("Clients", "clients", ...)`.
- [x] 6.3 Create `view_clients.templ`: header + search + a responsive grid of cards (logo/monogram, Title-Cased name, dialect badge, status badge: emerald Connected / amber Not Configured / slate Not Installed); whole card links to `/dashboard/clients/{id}`.
- [x] 6.4 Add the **Clients** nav item to `view_layout.templ` (Lucide SVG icon, e.g. `terminal`; between API Keys and Settings; `activeTab == "clients"`).
- [x] 6.5 Tests: list renders one card per registered client; badge mapping for each status; empty-registry case is clean.

## 7. Dashboard: client detail editor

- [x] 7.1 Add `GET /dashboard/clients/{id}` → `handleClientDetailView`; build `ClientDetailPageData` from `clients.Get(id)` + extended `Detect()` (current endpoint, masked key, slot values) + routable models for the dialect.
- [x] 7.2 Create `view_client_detail.templ` matching the wireframe: in-page client switcher; **Select endpoint** dropdown (gateway dialect endpoints, default derived); read-only **Current**; masked **API key**; per-slot model pickers from `ModelSlots()` with `[Select model]` + `[×]` clear; **Context window** field; deferred toggles (Filter naming, Web search/Exa MCP) rendered disabled/greyed.
- [x] 7.3 Actions row: **Apply**, **Reset**, **Manual config**. **Manual config** reveals a read-only snippet panel (`env.ANTHROPIC_BASE_URL = …`) for the adapter.
- [x] 7.4 Regenerate templ (`templ generate`); tests: editor renders live state from `Detect()`; endpoint dropdown defaults correctly; pickers generated from declared slots; unknown client id → redirect/error.

## 8. Dashboard: install preview / confirm / one-time reveal

- [x] 8.1 Add `POST /dashboard/clients/{id}/plan` → builds `clients.InstallRequest` from the form, calls `Installer.Plan`, returns the preview (Alpine modal) — no writes.
- [x] 8.2 Add `POST /dashboard/clients/{id}/apply` → on confirm, calls `Installer.Apply`; if a key was minted, render a one-time reveal result page (copy control), NOT a redirect; never place the key in a URL/flash/log.
- [x] 8.3 Tests (handler): plan writes nothing; decline path mints no key and writes nothing; apply-with-mint reveals the key once and persists a scoped key; apply-with-reuse writes the caller token and mints nothing; masked/secret fields never appear in redirects or JSON.

## 9. Dashboard: uninstall (Reset)

- [x] 9.1 Add `POST /dashboard/clients/{id}/reset` → confirm, then `adapter.Reset()`; redirect to the detail view with a flash; only injected fields removed.
- [x] 9.2 Tests: reset removes injected fields and preserves unrelated user settings; declining leaves config unchanged.

## 10. Spec capability rename + verify

- [x] 10.1 After implementation, rename the main spec capability folder `openspec/specs/agent-install` → `openspec/specs/clients-install`; update the in-file terminology; reconcile the change's `specs/agent-install/` delta path if required by the tooling.
- [x] 10.2 `openspec validate serve-clients-on-dashboard` passes; resolve any spec drift.
- [x] 10.3 `gofmt -w .`, `go build ./...`, `go test ./...` all green.
- [x] 10.4 Manual end-to-end: open Clients → card badges correct → open Claude Code → editor shows live state → Apply preview → confirm → minted key shown once → client config written and re-detected as Connected; Reset returns it to Not Configured.
