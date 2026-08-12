## Why

Configuring models manually in `config.json` via routing chains is tedious, prone to conflicts, and makes it hard to manage available models. We need a way to easily fetch, cache, and select models via the CLI, and simplify the routing architecture by maintaining a whitelist of models per provider.

## What Changes

- Add a `Models []string` whitelist array to the `Provider` struct in `config.json`.
- Simplify the Router to natively intercept prefixed requests (`provider:model`, e.g., `openai:gpt-4o`), bypassing the need to generate explicit manual routes in `config.json`. Requests without a provider prefix will be rejected.
- Fetch model catalogs from provider APIs directly, falling back to a global cache fetched from `https://models.dev/api.json`.
- Cache the fallback catalog locally at `~/.tinyroute/cache/api.json` with a 12-hour TTL and atomic writes.
- Add a nested `model` CLI command group under `provider` (`tinyroute provider model`) containing:
  - `add`: Interactive multi-select via `pterm` to whitelist models from the fetched catalog.
  - `list` / `ls`: List currently whitelisted models.
  - `remove` / `rm`: Remove a model from the whitelist.
  - `test`: Run an individual health probe against a specific whitelisted model.

## Capabilities

### New Capabilities
- `provider-model-management`: Ability to fetch, cache, whitelist, and test specific models per provider via the CLI, alongside direct prefixed routing (`provider:model`) without explicit route chaining.

### Modified Capabilities
- `core-routing`: The routing logic is changing to prioritize direct provider lookups based on the `provider:model` prefix and validating against the provider's whitelist.

## Impact

- `internal/config/topology.go`: `Provider` struct is modified.
- `internal/route/router.go`: Routing logic is updated to parse prefixes and check whitelists before falling back to manual chains, rejecting unprefixed requests.
- `internal/cli/commands.go`: Addition of the nested `model` subcommands and their handlers.
- `internal/cli/interactive/prompts.go`: Addition of `MultiSelect` using `pterm.DefaultInteractiveMultiselect`.
- `config.json` payload structures will be cleaner moving forward, replacing large `routes` arrays with simple `models` arrays on providers.
