## 1. Core Config & Types Updates

- [x] 1.1 Add `Models []string` field to `Provider` struct in `internal/config/topology.go`

## 2. Interactive CLI Components

- [x] 2.1 Implement `MultiSelect` in `internal/cli/interactive/prompts.go` using `pterm.DefaultInteractiveMultiselect`

## 3. Caching & Fetching Logic

- [x] 3.1 Implement fetching logic for the `models.dev/api.json` fallback catalog
- [x] 3.2 Implement logic to query provider-specific `/v1/models` APIs directly
- [x] 3.3 Implement `LoadOrRefreshCatalog` logic for atomic caching of the fallback catalog at `~/.tinyroute/cache/api.json` with `.sha256` checksum validation

## 4. Nested CLI Commands (`tinyroute provider model`)

- [x] 4.1 Update `cmdProvider` in `internal/cli/commands.go` to include the nested `model` command group
- [x] 4.2 Implement `tinyroute provider model list <provider>` to show the whitelist
- [x] 4.3 Implement `tinyroute provider model remove <provider> <model>` to delete a whitelisted model
- [x] 4.4 Implement `tinyroute provider model add <provider>` to orchestrate fetching catalogs, invoking `MultiSelect`, and saving choices to config
- [x] 4.5 Implement `tinyroute provider model test <provider> <model>` to run health probes against individual models

## 5. Router Updates

- [x] 5.1 Modify `Resolve` in `internal/route/router.go` to parse the `provider:model` syntax
- [x] 5.2 Implement direct O(1) resolution by checking the requested provider's `Models` whitelist
- [x] 5.3 Enforce strict prefixing by rejecting unprefixed requests that do not match existing manual `Routes` configurations

## 6. Interactive-First Convention & Documentation

- [x] 6.1 Apply the interactive-first pattern (Decision 5) to `provider model` handlers (`add`/`remove`/`list`/`test`): replace `if NArg < 1 { return error }` with prompt-when-absent (`Select`/`MultiSelect` from live state) + clear non-TTY error; single-candidate auto-select; empty-list informational exit
- [x] 6.2 Create `.claude/rules/cli-interactivity.md` documenting the interactive-first principle and applied rules
- [x] 6.3 Create root `CLAUDE.md` as a short entry index pointing into `.claude/rules/` (including the new `cli-interactivity.md`)

