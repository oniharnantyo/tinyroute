# Clients Install Delta

## MODIFIED Requirements

### Requirement: Agent base URL derivation

The system SHALL derive each agent's base URL from the gateway listen address (`svc.Listen`) and the agent's dialect: `anthropic` SHALL map to `http://<listen>/anthropic`; `openai` and `openai-responses` SHALL map to `http://<listen>/openai/v1`; `gemini` SHALL map to `http://<listen>/gemini`. The `openai` URL family serves both `/v1/chat/completions` and `/v1/responses` under the shared `/openai` prefix — the agent's client selects the endpoint; the adapter MUST NOT invent a separate `openai-responses` path prefix. A `--base-url` flag SHALL override the derived value.

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

On install, the system SHALL obtain the auth token for the agent config from one of two sources, chosen by the user:

1. **Mint** — generate a new `tr_live_` key via the existing keystore, plaintext shown once. Minted keys are not scoped: they carry no per-dialect allow-list.
2. **Reuse** — use a caller-supplied token (entered by the user, or passed via `--api-key`), written as-is; no key is minted or persisted.

In interactive mode the flow SHALL prompt the user to choose between minting and reusing a token (reuse entry collected with masked input); the CLI reuse path takes the token string from the user or `--api-key` — it does not replay stored keys. `--api-key` SHALL pre-select reuse and skip the prompt; a non-interactive run without `--api-key` SHALL default to minting. The chosen plaintext SHALL be written into the agent config and, if minted, reported exactly once. When minting, the key's display `Name` SHALL default to `agent-<id>` and the user MAY override it via an interactive prompt or `--name`.

#### Scenario: Interactive choice to mint
- **WHEN** `agent install claude` runs interactively and the user chooses "mint"
- **THEN** a new key is appended to keys.json and its plaintext is written into ~/.claude/settings.json

#### Scenario: Interactive choice to reuse a token
- **WHEN** `agent install claude` runs interactively and the user chooses "reuse" and enters a token
- **THEN** no key is minted or persisted and the entered token is written into the agent config

#### Scenario: Caller-supplied token via flag
- **WHEN** `agent install claude --api-key my-token` runs
- **THEN** no key is minted and `my-token` is written into the agent config

#### Scenario: Non-interactive without flag mints
- **WHEN** `agent install claude --force` runs (non-interactive, no `--api-key`)
- **THEN** a new key is minted and written into the agent config

#### Scenario: Minted key is named
- **WHEN** `agent install claude --name claude-laptop` mints a key
- **THEN** the persisted key record has `Name = "claude-laptop"` (default `agent-claude` when `--name` is omitted)
