# Interactive CLI Refactoring

## Why

The tinyroute CLI currently lacks interactive user experience features that are standard in modern CLI tools:
- No confirmation prompts for destructive operations (e.g., `tinyroute keys revoke` immediately disables keys without confirmation)
- No guided setup wizard for initial configuration
- Unmasked credential input via stdin (security concern)
- No interactive selection from provider presets
- No progress indicators for long-running operations

This limits usability for new users and creates opportunities for accidental destructive actions. Adding interactive features with **interactive mode as the default** will improve user experience while providing opt-out options for automation.

## What Changes

- Add **confirmation prompts** for destructive operations (`keys revoke`, `provider auth set`)
- Implement **masked password input** for sensitive credential entry
- Create **interactive selection menus** for provider presets
- Add **progress indicators** (spinners/progress bars) for long-running operations
- Build **guided setup wizard** for `tinyroute init`
- Make **interactive mode the DEFAULT** behavior for all commands
- Add **`--no-interactive` flag** to disable prompts (for scripts/automation)
- Implement **graceful degradation** for non-terminal environments (pipes, CI/CD)

**⚠️ BREAKING CHANGE:** Interactive mode is now ON by default. Existing scripts that expect immediate execution must add `--no-interactive` or `--force` flags.

## Capabilities

### New Capabilities
- `interactive-prompts`: Confirmation prompts, text input with validation, masked password entry
- `interactive-wizard`: Multi-step guided setup flow for initial configuration
- `progress-indicators`: Spinners and progress bars for long-running operations

### Modified Capabilities
- None - all changes are additive and opt-in

## Impact

**Affected Code:**
- `/internal/cli/commands.go` - Add interactive calls to existing commands
- `/internal/cli/cli.go` - Import interactive package, add `--interactive` flag
- `/go.mod` - Add `github.com/pterm/pterm` dependency

**New Package:**
- `/internal/cli/interactive/` - New package for interactive features
  - `prompts.go` - Core prompt wrappers (confirm, select, input, password)
  - `wizard.go` - Multi-step setup wizard
  - `progress.go` - Progress indicators

**Affected Commands:**
- `tinyroute init` - Interactive wizard runs by default
- `tinyroute keys revoke` - Confirmation prompt ON by default
- `tinyroute keys create` - Interactive name prompt ON by default
- `tinyroute provider add` - Interactive preset selection ON by default
- `tinyroute provider auth set` - Masked password prompt ON by default
- `tinyroute provider test` - Progress indicators ON by default
- `tinyroute compact` - Progress bar ON by default

**All commands support `--no-interactive` / `--force` to disable prompts.**

**Dependencies:**
- New: `github.com/pterm/pterm v0.12.70+` (actively maintained, purpose-built for CLI enhancement)

**Backward Compatibility:**
- ⚠️ **BREAKING CHANGE:** Interactive mode is now ON by default
- Existing scripts must add `--no-interactive` or `--force` to maintain non-interactive behavior
- `--no-interactive` flag explicitly disables all prompts
- `--force` flag acts as an alias for `--no-interactive`
- Non-terminal environments (pipes, CI/CD) automatically degrade to non-interactive
- **Migration required:** Update scripts to include `--no-interactive` or `--force` flags
