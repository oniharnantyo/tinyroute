## 1. Foundation: package, interface, registry

- [x] 1.1 Create `internal/agent/` package with `agent.go` defining the `Agent` interface (`ID`, `Name`, `Dialect`, `NeedsModel`, `Detect`, `Apply`, `Reset`) and the registry (`All()`, `Get(id)`) mirroring `internal/dialect/registry.go`
- [x] 1.2 Add `internal/agent/types.go` DTOs: `Status` (Installed, PointedAtTinyRoute, ConfigPath), `ApplyInput` (BaseURL, APIKey, Model), `Result` (Files, Key, Backup)
- [x] 1.3 Add `internal/agent/fileutil.go`: home-dir resolution, `backup(path) -> .tinyroute.bak`, and `atomicWrite(path, data, 0600)` (tmp+rename, reuse the `auth.WriteKeyFile` idiom)
- [x] 1.4 Add `internal/agent/agent_test.go` + `fileutil_test.go` covering registry lookup/miss and backup/atomic-write semantics

## 2. Format-family helpers + dependencies

- [x] 2.1 Add `github.com/pelletier/go-toml/v2` and `gopkg.in/yaml.v3` to `go.mod` (`go get`)
- [x] 2.2 Add `internal/agent/envjson.go`: parse JSON config to `map[string]any`, set nested env/keys, marshal back preserving unknown fields
- [x] 2.3 Add `internal/agent/tomlprovider.go`: parse TOML to map, set `[model_providers.<id>]` + root fields, marshal map back preserving unknown fields
- [x] 2.4 Add `internal/agent/yamlmerge.go`: parse YAML to map, mutate, marshal back (for hermes)
- [x] 2.5 Add tests asserting an arbitrary user config (JSON/TOML/YAML) survives parse→mutate→marshal with only the targeted fields changed

## 3. Verified exemplar adapters (anchor the interface)

- [x] 3.1 Implement `internal/agent/claude.go` from `9router/.../claude-settings/route.js`: write `env.ANTHROPIC_BASE_URL` + `env.ANTHROPIC_AUTH_TOKEN` to `~/.claude/settings.json`; reset removes only the injected env keys
- [x] 3.2 Add `internal/agent/claude_test.go` with golden fixtures: merge-into-existing, create-when-missing, idempotent re-apply, scoped reset
- [x] 3.3 Implement `internal/agent/codex.go` from `9router/.../codex-settings/route.js`: write `~/.codex/config.toml` (model, model_provider, `[model_providers.X]` base_url+`wire_api="responses"`, `[agents.subagent]`) and `~/.codex/auth.json` (`OPENAI_API_KEY`, `auth_mode="apikey"`)
- [x] 3.4 Add `internal/agent/codex_test.go` with golden fixtures across both files (merge, create, idempotent, scoped reset)
- [x] 3.5 Register claude + codex in the registry; verify `All()` returns them with correct dialects

## 4. CLI command group

- [x] 4.1 Implement `internal/cli/agent.go` `cmdAgent()` subcommand group (mirrors `cmdKeys` in `commands.go:858`): `install`, `status`, `uninstall`, each with the `--interactive/--no-interactive/--force` flag trio
- [x] 4.2 Implement `cmdAgentInstall`: interactive-first agent selection, `dialectBaseURL(svc.Listen, dialect)` derivation, mint scoped key via `auth.GenerateKey`/`loadKeyFile`/`auth.WriteKeyFile` (or honor `--api-key`), optional model pick from topology routes for `NeedsModel()` adapters, then `adapter.Apply`
- [x] 4.3 Implement `cmdAgentStatus`: per-adapter table (AGENT | INSTALLED | POINTED-AT-TINYROUTE | CONFIG PATH)
- [x] 4.4 Implement `cmdAgentUninstall`: `Confirm` guard (skip with `--force`) then `adapter.Reset()`
- [x] 4.5 Register `cmdAgent()` in `internal/cli/cli.go` root `Commands` (next to `cmdKeys`/`cmdProvider`/`cmdAuth`)
- [x] 4.6 Add `internal/cli/agent_test.go`: install/uninstall against a temp HOME (`t.Setenv`), missing-arg non-TTY error, status output; drive prompts via `interactive.SetCanPromptOverride`

## 5. Remaining adapters (extend the registry)

- [x] 5.1 Port `cline` (`~/.cline/data/globalState.json` + `secrets.json`, JSON-pair) + test
- [x] 5.2 Port `copilot` — confirm format from `9router/.../copilot-settings/route.js` + test
- [x] 5.3 Port `deepseek` (`~/.deepseek/config.toml`, TOML) + test
- [x] 5.4 Port `devin` — confirm format from `9router/.../devin-settings/route.js` + test
- [x] 5.5 Port `droid` (`~/.factory/settings.json`, env-JSON) + test
- [x] 5.6 Port `grok` (`~/.grok/config.toml`, TOML) + test
- [x] 5.7 Port `hermes` (`~/.hermes/config.yaml` + `.env`, YAML) + test
- [x] 5.8 Port `jcode` (`~/.jcode/config.toml`, TOML) + test
- [x] 5.9 Port `kilo` (`~/.local/share/kilo/auth.json` + `~/.config/Code/User/settings.json`, JSON-pair) + test
- [x] 5.10 Port `openclaw` (`~/.openclaw/openclaw.json`, env-JSON) — confirm dialect + test
- [x] 5.11 Port `opencode` (`~/.config/opencode/opencode.json`, JSON) — confirm dialect + test
- [x] 5.12 Verify `All()` returns all 13 adapters and none of the deferred ones (cowork, antigravity-mitm)

## 6. Integration, polish, verification

- [x] 6.1 End-to-end: `tinyroute serve` on a test port → `agent install claude` → send a Claude-Code-shaped POST to `/anthropic/v1/messages` with the issued key → assert `200`
- [x] 6.2 Confirm `cowork`/`antigravity-mitm` are absent from `agent status`/`agent install` picker
- [x] 6.3 `gofmt -w .` and reach 80% coverage on `internal/agent/` and `internal/cli/agent.go`
- [x] 6.4 Add a short `agent` section to `docs/ARCHITECTURE.md` and the CLI help text (deferred adapters noted)

## 7. Refinement: model selection, preview/confirm, auth choice, base-URL fix

Tracks the spec/design refinements in `specs/agent-install/spec.md` and `design.md` (D8/D9 + the
base-URL and tier corrections). These are additive on top of the shipped v1.

- [x] 7.1 `internal/agent/types.go`: add `ModelSlot{ID, Name, Kind, Required}` with `ModelSlotKind` (`SlotSingle`/`SlotMulti`) consts; add `ModelSlots map[string]string` plus `Model string` and `Models []string` to `ApplyInput` (matches the implemented shape)
- [x] 7.2 `internal/agent/agent.go`: add `ModelSlots() []ModelSlot` as a **required** method on the `Agent` interface (devin returns nil) — N.B. every adapter must implement it for the package to compile
- [x] 7.3 Per-adapter model slots per the spec's per-agent contract table: claude (fable/opus/sonnet/haiku + subagent, all optional singles); codex (model + subagent); cline/deepseek/hermes/kilo (single); copilot/droid/jcode/opencode/openclaw (multi, with active/subagent variants where applicable)
- [x] 7.4 CLI `routableModels(svc, dialect)` helper via `route.New(...).Models(dialect)`; `pickModel` (`Select`) and `pickModels` (`MultiSelect`) with skip for optional slots and free-text fallback when no routed models exist
- [x] 7.5 Wire model selection into `cmdAgentInstall`: multi/tiered via `ModelSlots`, single via `NeedsModel`, options sourced from routed models; honor `--model`
- [x] 7.6 Auth-token choice: interactive mint-vs-reuse prompt (reuse entry masked); `--api-key` pre-selects reuse and skips the prompt; non-interactive defaults to mint
- [x] 7.7 Preview + confirm gate: render preview (agent, base URL, key source, selected model(s), target path, backup note); `interactive.Confirm`; on decline print "Aborted. No changes made." with no key minted and no file written
- [x] 7.8 Defer key mint until after confirm; then mint scoped key (or use supplied token) and `adapter.Apply`
- [x] 7.9 `claude.go`: write selected `ANTHROPIC_DEFAULT_FABLE_MODEL`/`_OPUS_`/`_SONNET_`/`_HAIKU_` + `CLAUDE_CODE_SUBAGENT_MODEL` from `ApplyInput.ModelSlots`; unselected tiers left untouched
- [x] 7.10 Base-URL fixes: `dialectBaseURL` maps `openai-responses` → `/openai/v1` (shared family, not `/openairesponses`); `codex.Dialect()` returns the real `openai-responses` so the minted key scope (`Allow=["openai-responses:*"]`) matches the inbound surface; `claude` writes `ANTHROPIC_BASE_URL` WITHOUT `/v1` (Claude Code appends `/v1/messages`)
- [x] 7.11 Tests: per-adapter slot wiring; routed-models picker (with and without routed models); preview-abort writes nothing; claude tier writing (selected set written, unselected omitted); base-URL value per dialect (anthropic→/anthropic, openai & openai-responses→/openai/v1)
- [x] 7.12 `gofmt -w .` and 80% coverage on changed files (`internal/agent/`, `internal/cli/agent.go`)
- [x] 7.13 `copilot.go`: set `vendor: "customendpoint"` (not `"azure"`) and drop the `#models.ai.azure.com` URL fragment per official VS Code language-model docs
- [x] 7.14 `kilo.go`: target the documented `kilo.json` provider block instead of the undocumented `kilocode.*` VS Code keys
- [x] 7.15 Key naming on mint: interactive prompt for the key `Name` (default `agent-<id>`) + `--name` flag in `cmdAgentInstall`; surface in `keys list`
- [ ] 7.16 Human-verify the unverified adapters before release — `deepseek`, `grok`, `hermes`, `jcode`, `openclaw` (9router-derived configs, no trustworthy official docs found); confirm each tool exists and its schema matches, or gate/disable
