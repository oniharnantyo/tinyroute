## Context

Currently, the `config.json` topology file maps models to providers via a dense `routes` array. Each entry matches an inbound model string to an outbound provider chain. While flexible, this becomes verbose as we support more models. Furthermore, we lack a CLI mechanism to query, whitelist, and interactively manage supported models for each provider natively. 

## Goals / Non-Goals

**Goals:**
- Shift model association from manual `routes` chaining to a strict provider `models` whitelist array.
- Support strict prefix routing (`provider:model`) to eliminate namespace collisions across providers.
- Implement a caching layer for `models.dev/api.json` with a 12-hour TTL and atomic writes to avoid fetching over the network on every invocation.
- Introduce interactive CLI commands to manage and test whitelisted models per provider.

**Non-Goals:**
- Completely removing the `Routes` structure in `config.json` (it may still be needed for advanced fallback/alias chains, though prefixed direct routing takes priority).
- Modifying how inference requests payload structures are built beyond resolving the proper provider.

## Decisions

**1. Strict Prefix Routing**
- **Decision:** The router will reject inbound models that do not contain a `provider:` prefix, except if explicit legacy route rules match. If the requested model is `openai:gpt-4o`, the router isolates `openai` as the provider and `gpt-4o` as the model, ensuring an O(1) provider lookup. 
- **Rationale:** Prevents collisions, simplifies logic, and eliminates the need for dynamic provider resolution across large whitelists.

**2. Whitelist Data Structure**
- **Decision:** Providers in `config.json` will gain a `Models []string` field representing the whitelist.
- **Rationale:** Straightforward exact-string-match checking. No globs to reduce regex parsing overhead.

**3. Caching Strategy**
- **Decision:** The fallback catalog (`models.dev/api.json`) will be cached in `~/.tinyroute/cache/api.json` with an atomic rename (`.tmp` write then `Rename`) and a `.sha256` checksum file. TTL is 12 hours. Provider-specific APIs (like `/v1/models`) will be queried live during the `add` command, with no persistent caching.
- **Rationale:** Provider APIs are typically fast enough for live querying. The global fallback handles massive lists and benefits greatly from caching. Atomic writes ensure corrupted states do not occur during interrupted CLI commands.

**4. Interactive MultiSelect**
- **Decision:** Utilize `pterm.DefaultInteractiveMultiselect` in `internal/cli/interactive/prompts.go` to provide a robust checklist selection for models. (pterm v0.12.83 enables `Filter` + `[type to search]` by default, so large catalogs remain usable.)

**5. Interactive-First Command Convention**
- **Decision:** Adopt "interactive-first, args-as-escape-hatch" as the CLI convention. Commands must not *require* typed positional arguments: when a required value is absent and a TTY is attached (`interactive.CanPrompt()`), gather it via `Select`/`MultiSelect` from live system state; typed args remain an optional shortcut for scripts and non-interactive runs. `provider add` is the reference implementation.
- **Applied rules:**
  - Triggering: honor args/flags if present; prompt only when absent (backward compatible).
  - Non-TTY + missing arg → clear error naming the required value; never guess.
  - Single candidate → auto-select and skip the prompt.
  - Empty source list (e.g. `model remove` on a provider with no whitelisted models) → informational exit, not an empty picker.
  - Pickers draw from real state (topology providers, a provider's `Models` whitelist), so selections are valid by construction — never offer free-text where a list exists.
  - Honor `--no-interactive` / `--force` on commands that define them.
  - Scope: prove on `provider model *`, then propagate to the rest of the tree.
- **Rationale:** Trades "type a string we then validate" for "pick from valid state," removing the `usage:` dead-end and the typo failure class while keeping the CLI scriptable.
- **Documentation:** After implementation, persist the convention as `.claude/rules/cli-interactivity.md` and add a short root `CLAUDE.md` entry doc that points into `.claude/rules/`. (Chosen over a root `AGENTS.md` to avoid duplicating the existing `.claude/rules/` convention layer.)

## Risks / Trade-offs

- **Risk: Breaking generic clients.** Clients that send raw model names (e.g., `gpt-4o` instead of `openai:gpt-4o`) will fail.
  - *Mitigation:* Document clearly that `tinyroute` prioritizes strict routing. Advanced users can still manually craft a wildcard route in `Routes` if they need legacy fallback behavior.
- **Risk: Interrupted caching.** 
  - *Mitigation:* The atomic rename strategy with checksums eliminates partial read/write issues.
