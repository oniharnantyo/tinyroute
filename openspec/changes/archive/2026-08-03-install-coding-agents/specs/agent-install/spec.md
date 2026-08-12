## ADDED Requirements

### Requirement: Agent adapter registry
The system SHALL provide a registry of coding-agent adapters. Each adapter SHALL expose a stable
`id`, a human-readable `name`, and the `dialect` it speaks. The registry SHALL be enumerable
(`All()`) and resolvable by id (`Get(id)`).

#### Scenario: Enumerate supported agents
- **WHEN** the registry is queried for all adapters
- **THEN** it returns claude, codex, cline, copilot, deepseek, devin, droid, grok, hermes, jcode, kilo, openclaw, and opencode

#### Scenario: Resolve a known agent
- **WHEN** the registry is queried by id "codex"
- **THEN** it returns the codex adapter whose dialect is "openai-responses"

#### Scenario: Unknown agent is not found
- **WHEN** the registry is queried by id "unknown"
- **THEN** it returns no adapter and an error

#### Scenario: Deferred mechanisms are absent
- **WHEN** the registry is enumerated
- **THEN** cowork and antigravity-mitm are not present, because MCP-registry and MITM-proxy mechanisms are out of scope

### Requirement: Interactive-first agent selection
`agent install` SHALL accept an optional positional agent id. When the id is absent and a TTY is
attached, it SHALL prompt the user to select from the registry. When exactly one candidate exists
it SHALL auto-select without prompting. When the id is absent and no TTY is attached, it SHALL
fail with an error naming the required value.

#### Scenario: Supplied arg is honored
- **WHEN** `agent install claude` is run
- **THEN** the claude adapter is selected without prompting

#### Scenario: Missing arg in a TTY prompts
- **WHEN** `agent install` is run with no arg in a terminal
- **THEN** the user is prompted to select an agent from the registry

#### Scenario: Missing arg without TTY errors
- **WHEN** `agent install` is run with no arg and stdin is not a TTY
- **THEN** the command fails with an error stating the agent is required

### Requirement: Agent base URL derivation
The system SHALL derive each agent's base URL from the gateway listen address (`svc.Listen`) and
the agent's dialect: `anthropic` SHALL map to `http://<listen>/anthropic`; `openai` and
`openai-responses` SHALL map to `http://<listen>/openai/v1`; `gemini` SHALL map to
`http://<listen>/gemini`. The `openai` URL family serves both `/v1/chat/completions` and
`/v1/responses` under the shared `/openai` prefix — the agent's client selects the endpoint; the
adapter MUST NOT invent a separate `openai-responses` path prefix. The dialect *name* still drives
auth key scoping (e.g. `Allow=["openai-responses:*"]`), distinct from the URL path family. A
`--base-url` flag SHALL override the derived value.

#### Scenario: Anthropic base URL
- **WHEN** installing the claude adapter with listen 127.0.0.1:8787
- **THEN** the configured base URL is http://127.0.0.1:8787/anthropic

#### Scenario: OpenAI-responses base URL
- **WHEN** installing the codex adapter with listen 127.0.0.1:8787
- **THEN** the configured base URL is http://127.0.0.1:8787/openai/v1

#### Scenario: Explicit override
- **WHEN** `agent install claude --base-url https://gateway.example/internal` is run
- **THEN** the configured base URL is the provided value, not the derived one

### Requirement: API key provisioning on install
On install, the system SHALL obtain the auth token for the agent config from one of two sources,
chosen by the user:
1. **Mint** — generate a new `tr_live_` key scoped to the agent's dialect
   (`Key.Allow = ["<dialect>:*"]`), persisted via the existing keystore, plaintext shown once.
2. **Reuse** — use a caller-supplied token (entered by the user, or passed via `--api-key`),
   written as-is; no key is minted or persisted.

In interactive mode the flow SHALL prompt the user to choose between minting and reusing a token
(reuse entry collected with masked input). Because tinyroute stores only key digests, reuse
cannot replay an existing key's plaintext — the user supplies the token string. `--api-key`
SHALL pre-select reuse and skip the prompt; a non-interactive run without `--api-key` SHALL
default to minting. The chosen plaintext SHALL be written into the agent config and, if minted,
reported exactly once. When minting, the key's display `Name` SHALL default to `agent-<id>` and
the user MAY override it via an interactive prompt or `--name`.

#### Scenario: Interactive choice to mint
- **WHEN** `agent install claude` runs interactively and the user chooses "mint"
- **THEN** a new key is appended to keys.json with Allow `["anthropic:*"]` and its plaintext is written into ~/.claude/settings.json

#### Scenario: Interactive choice to reuse a token
- **WHEN** `agent install claude` runs interactively and the user chooses "reuse" and enters a token
- **THEN** no key is minted or persisted and the entered token is written into the agent config

#### Scenario: Caller-supplied token via flag
- **WHEN** `agent install claude --api-key my-token` runs
- **THEN** no key is minted and `my-token` is written into the agent config

#### Scenario: Non-interactive without flag mints
- **WHEN** `agent install claude --force` runs (non-interactive, no `--api-key`)
- **THEN** a new scoped key is minted and written into the agent config

#### Scenario: Minted key is named
- **WHEN** `agent install claude --name claude-laptop` mints a key
- **THEN** the persisted key record has `Name = "claude-laptop"` (default `agent-claude` when `--name` is omitted)

### Requirement: Model selection
Each adapter SHALL declare the model selections it supports — each shaped as a single pick or a
multi-list, and each optional or required. The install flow SHALL prompt for each declared
selection before writing config: a single selection via `Select`, a multi-list via `MultiSelect`,
with options sourced from the models tinyroute can route for the agent's dialect. Selections SHALL
be skippable unless declared required. When no routed models exist, a required selection SHALL
fall back to free-text entry and an optional selection SHALL be skipped. Adapters that declare no
model selections SHALL skip model prompting entirely. A `--model` flag SHALL pre-fill any single
selection and skip its prompt.

#### Scenario: Single-model agent uses routed models
- **WHEN** installing an agent that declares one required single-model slot, interactively
- **THEN** the user is offered the models routable on that agent's dialect and the selection is written to the agent's model field

#### Scenario: Multi-list agent selects several models
- **WHEN** installing an agent that declares a multi-list slot (e.g. copilot), interactively
- **THEN** the user is offered a multi-select of routable models and the chosen list is written to the agent's config

#### Scenario: Optional slot may be skipped
- **WHEN** the user skips an optional model slot
- **THEN** the agent's corresponding model field is left untouched by tinyroute

#### Scenario: Agent without model selections skips prompting
- **WHEN** installing an agent that declares no model slots (e.g. devin)
- **THEN** no model prompt is shown and only base URL + key (if any) are configured

### Requirement: Per-agent model selection contract
Each adapter SHALL map its declared model selections to the agent's documented config fields. The
contract below is derived from 9router's per-agent handlers
(`9router/src/app/api/cli-tools/<id>-settings/route.js`):

| Agent | Shape | Slot → config field |
|---|---|---|
| claude | tiered (named singles) + subagent | fable → `env.ANTHROPIC_DEFAULT_FABLE_MODEL`; opus → `env.ANTHROPIC_DEFAULT_OPUS_MODEL`; sonnet → `env.ANTHROPIC_DEFAULT_SONNET_MODEL`; haiku → `env.ANTHROPIC_DEFAULT_HAIKU_MODEL`; subagent → `env.CLAUDE_CODE_SUBAGENT_MODEL` (all optional) |
| codex | single + subagent | model → `model` (required); subagent → `[agents.subagent].model` |
| cline | single | model → `globalState.openAiModelId` |
| copilot | multi-list | models → entry in `chatLanguageModels.json` with `vendor: "customendpoint"` (NOT `"azure"`), no `#models.ai.azure.com` URL fragment (required, ≥1) |
| deepseek | single | model → `model` |
| devin | none | no config; auth is binary-managed (`devin auth login`) — install is detection-only |
| droid | multi-list + active | models → per-model entries; active → the active model |
| grok | single + subagents | model → `model`; subagents → subagent model list |
| hermes | single | model → `model.default` (YAML) |
| jcode | multi-list | models → `default_model` (defaults to first selected) |
| kilo | single | model → the documented `kilo.json` provider block (the `kilocode.*` VS Code keys are undocumented; avoid) |
| openclaw | single + per-agent map | model → `agents.defaults.model`; agentModels → per-agent `9router/<model>` overrides |
| opencode | multi-list + active + subagent | models → provider model list; active → `config.model`; subagent → subagent model |

#### Scenario: Claude tiered selections
- **WHEN** claude is installed with opus = `A` and haiku = `C` (sonnet skipped)
- **THEN** settings.json env sets `ANTHROPIC_DEFAULT_OPUS_MODEL` = `A` and `ANTHROPIC_DEFAULT_HAIKU_MODEL` = `C`, and tinyroute adds no sonnet entry

#### Scenario: Codex model and subagent
- **WHEN** codex is installed with model = `M` and subagent = `S`
- **THEN** config.toml sets `model` = `M` and `[agents.subagent].model` = `S`

#### Scenario: Copilot multi-list
- **WHEN** copilot is installed with models `[A, B]`
- **THEN** the 9Router entry in chatLanguageModels.json contains model entries for both `A` and `B`

#### Scenario: Devin writes no model config
- **WHEN** devin is installed
- **THEN** no model field is written and the flow reports detection only, directing the user to `devin auth login`

### Requirement: Preview and confirmation
In interactive mode, the install flow SHALL render a preview of the planned configuration before
writing anything, including: the agent, the resolved base URL, whether a key will be minted or a
supplied key used, the selected model(s), the target config path(s), and whether an existing file
will be backed up. The flow SHALL then require explicit confirmation before applying. If the user
does not confirm, no key SHALL be minted and no file SHALL be written. Non-interactive runs
(`--no-interactive` / `--force`) SHALL skip the preview and confirmation and apply directly.

#### Scenario: Abort writes nothing
- **WHEN** the user declines confirmation at the preview
- **THEN** no API key is minted, no config file is modified, and no backup is created

#### Scenario: Confirm applies the configuration
- **WHEN** the user confirms the preview
- **THEN** the key (if minted) is persisted and the agent config is written

#### Scenario: Non-interactive skips preview
- **WHEN** `agent install claude --force` runs
- **THEN** no preview or confirmation prompt is shown and the configuration is applied directly

### Requirement: Safe config writes
Each adapter SHALL, before writing: back up any existing target file to
`<file>.tinyroute.bak`; merge tinyroute's fields into the existing content preserving all
unrelated user fields; write atomically via temp-file + rename at POSIX mode `0600`. A second
install of the same agent SHALL be idempotent.

#### Scenario: Existing config is merged, not overwritten
- **WHEN** install writes to an agent config that already contains unrelated user settings
- **THEN** the unrelated settings are preserved and only tinyroute's fields are set or updated

#### Scenario: Atomic write with restrictive permissions
- **WHEN** a config file is written
- **THEN** it is written via temp-file + rename and has mode `0600`

#### Scenario: Idempotent re-install
- **WHEN** the same agent is installed twice
- **THEN** the resulting config equals that of a single install, with no duplicated fields

#### Scenario: Existing file is backed up
- **WHEN** install overwrites an existing agent config
- **THEN** a backup copy exists at `<file>.tinyroute.bak` before the write completes

### Requirement: Per-agent config contract
Each adapter SHALL write the agent's documented configuration to its well-known home-directory
path(s). The claude adapter SHALL write `env.ANTHROPIC_BASE_URL` and `env.ANTHROPIC_AUTH_TOKEN`
to `~/.claude/settings.json`, and for each tier/role the user selected, the corresponding env var
(`ANTHROPIC_DEFAULT_FABLE_MODEL`, `ANTHROPIC_DEFAULT_OPUS_MODEL`, `ANTHROPIC_DEFAULT_SONNET_MODEL`,
`ANTHROPIC_DEFAULT_HAIKU_MODEL`, or `CLAUDE_CODE_SUBAGENT_MODEL`); those the user did not select
SHALL be left untouched. The codex
adapter SHALL write `model`, `model_provider`, the `[model_providers.<id>]` table (with `base_url`
and `wire_api = "responses"`) and the `[agents.subagent]` table to `~/.codex/config.toml`, and
`OPENAI_API_KEY` + `auth_mode = "apikey"` to `~/.codex/auth.json`.

#### Scenario: Claude Code config with selected tiers
- **WHEN** the claude adapter applies with base URL `U`, key `K`, opus = `A`, and haiku = `C` (fable, sonnet, subagent unset)
- **THEN** ~/.claude/settings.json env contains `ANTHROPIC_BASE_URL` = `U`, `ANTHROPIC_AUTH_TOKEN` = `K`, `ANTHROPIC_DEFAULT_OPUS_MODEL` = `A`, and `ANTHROPIC_DEFAULT_HAIKU_MODEL` = `C`, and tinyroute adds no fable / sonnet / subagent entries

#### Scenario: Codex config across two files
- **WHEN** the codex adapter applies with base URL `U`, key `K`, and model `M`
- **THEN** ~/.codex/config.toml contains `model` = `M`, a model_providers table with `base_url` = `U` and `wire_api` = "responses", and ~/.codex/auth.json contains `OPENAI_API_KEY` = `K` and `auth_mode` = "apikey"

### Requirement: agent status reporting
`agent status` SHALL report, for each adapter: whether its config file is present (installed),
whether its base URL field references the tinyroute gateway (pointed-at-tinyroute), and the
config file path.

#### Scenario: Unconfigured agent
- **WHEN** `agent status` runs and an agent's config file does not exist
- **THEN** that agent reports installed = false and pointed-at-tinyroute = false

#### Scenario: Configured agent
- **WHEN** an agent's config contains a base URL referencing the gateway listen address
- **THEN** that agent reports pointed-at-tinyroute = true

### Requirement: agent uninstall
`agent uninstall <agent>` SHALL remove only the fields tinyroute injected, preserving all other
user settings, after a confirmation prompt. `--force` SHALL skip the confirmation. Missing-arg
behavior SHALL follow the interactive-first selection rule.

#### Scenario: Only injected fields are removed
- **WHEN** `agent uninstall claude` runs on a config containing tinyroute fields and unrelated user fields
- **THEN** the tinyroute fields are removed and the unrelated fields remain

#### Scenario: Confirmation guard
- **WHEN** `agent uninstall claude` runs in a TTY without `--force`
- **THEN** the user is asked to confirm before any change is made
