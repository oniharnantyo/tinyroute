## Context

The tinyroute CLI is built using `github.com/urfave/cli/v3` and currently has minimal interactive capabilities. All user input comes from command-line flags, arguments, or unmasked stdin reading. The codebase follows Go best practices with clean separation of concerns across packages (auth, config, preset, dialect, etc.).

Current limitations:
- Destructive operations like `keys revoke` execute immediately without confirmation
- Credential input via `readSecretFromStdin()` is unmasked (security concern)
- No guided setup for new users - they must manually edit config files
- No progress feedback during long-running operations (provider testing, blob compaction)
- Provider selection requires knowing preset names beforehand

## Goals / Non-Goals

**Goals:**
- Add interactive prompts for confirmation, input validation, and credential masking
- Implement a guided setup wizard for initial configuration
- Provide progress indicators for long-running operations
- **Make interactive mode the DEFAULT behavior** for better UX
- Keep implementation simple and maintainable
- Provide `--no-interactive` flag for automation compatibility

**Non-Goals:**
- Full terminal UI (TUI) applications - just enhance existing CLI
- Complex multi-screen wizards - single linear flow only
- Backward compatibility without migration - **this is a BREAKING CHANGE**

## Decisions

### Library Choice: PTerm over Bubble Tea

**Decision:** Use `github.com/pterm/pterm` for interactive features

**Rationale:**
- PTerm is purpose-built for CLI enhancement (confirmation, input, progress bars)
- Simpler API vs Bubble Tea's Elm Architecture pattern
- Lower learning curve and less boilerplate for our use cases
- All features we need are built-in (InteractiveConfirm, InteractiveTextInput, Spinner, Progressbar)
- Actively maintained and cross-platform compatible

**Alternatives Considered:**
- Custom implementation with bufio - rejected due to reinventing wheel and limited features
- Bubble Tea - rejected as overkill for simple prompts and progress indicators

### Architecture: New internal/cli/interactive Package

**Decision:** Create dedicated `interactive` package with three focused files

**Rationale:**
- Clean separation of interactive logic from command implementations
- Easy to test in isolation
- Follows Go convention of one package per concern
- Allows future expansion without bloating commands.go

**Package Structure:**
```
internal/cli/interactive/
├── prompts.go      # Confirm, Select, Input, Password wrappers with terminal detection
├── wizard.go       # Multi-step setup wizard
└── progress.go     # Spinner and progress bar wrappers
```

### Terminal Detection and Graceful Degradation

**Decision:** All interactive functions check terminal capability and auto-degrade

**Rationale:**
- Prevents hanging in non-interactive contexts (CI/CD, pipes, cron jobs)
- No user intervention needed - system adapts automatically
- Maintains backward compatibility without requiring flags

**Implementation:**
```go
func CanPrompt() bool {
    return isTerminal(os.Stdout.Fd()) && isTerminal(os.Stdin.Fd())
}
```

### Default Interactive Mode

**Decision:** Interactive mode is ON by default, with `--no-interactive` to disable

**Rationale:**
- Best user experience - prompts prevent accidental destructive actions
- Discoverability - users see all interactive features immediately
- Standard CLI behavior - most modern CLIs prompt by default
- Scripts can explicitly opt-out with `--no-interactive`

**Flag Strategy:**
- **Default (no flags):** Interactive mode ON for terminals
- `--no-interactive`: Explicitly disables all prompts (for scripts/automation)
- `--force`: Alias for `--no-interactive` (maintains familiarity)
- Non-terminal environments (pipes, CI/CD): Auto-degrade to non-interactive

**Breaking Change Consideration:**
- Scripts that expect immediate execution WILL BREAK
- Migration required: add `--no-interactive` or `--force` to script commands
- Trade-off accepted for significantly improved human UX

### Progress Indicators

**Decision:** Use PTerm Spinner for indeterminate progress, Progressbar for determinate

**Rationale:**
- Spinner: unknown duration operations (provider testing, network calls)
- Progressbar: known quantity operations (blob compaction with file count)
- Visual feedback keeps users informed during long operations
- Simple integration with existing code

### Masked Password Input

**Decision:** Replace `readSecretFromStdin()` with PTerm's InteractiveTextInput.WithMask("*")

**Rationale:**
- Standard security practice for credential input
- Prevents shoulder-surfing and screen sharing exposure
- PTerm provides cross-platform masking (Windows, Linux, macOS)
- Simple drop-in replacement

## Risks / Trade-offs

**[Risk] Breaking existing scripts**
→ **CRITICAL:** All scripts expecting immediate execution will now hang waiting for input
→ Mitigation: Provide clear migration guide and deprecation notice. Scripts must add `--no-interactive` or `--force`.

**[Risk] User confusion about new default behavior**
→ Mitigation: Clear documentation, release notes, and help text explaining the change and how to opt-out.

**[Risk] PTerm dependency becomes unmaintained**
→ Mitigation: PTerm is actively maintained with regular releases. If abandoned, simple API allows switching to alternative (1-2 days work).

**[Risk] Interactive prompts hang in CI/CD pipelines**
→ Mitigation: Terminal detection (isatty) automatically degrades. CI environments return false for isatty checks, preventing prompts.

**[Trade-off] Additional dependency increases binary size**
→ Mitigation: PTerm is lightweight (~100KB compressed). Benefits outweigh size cost for CLI tools.

**[Trade-off] Testing complexity for interactive features**
→ Mitigation: Use bufio/buffers for automated testing. Terminal detection can be mocked. No complex UI testing required.

## Migration Plan

**⚠️ Migration REQUIRED** - Breaking change for existing scripts

**Deployment Strategy:**
1. Add `github.com/pterm/pterm` to go.mod
2. Create `internal/cli/interactive` package with default interactive=true flags
3. Add `--no-interactive` flag to all interactive commands
4. Update documentation with migration guide
5. **Release notes:** Clearly announce breaking change and migration path
6. Deploy and monitor for issues

**User Migration Steps:**
1. Identify all scripts using tinyroute commands
2. Add `--no-interactive` or `--force` to each command
3. Test scripts with new flags
4. Update CI/CD pipelines if affected
5. Monitor for prompt-related hangs in automation

**Rollback Strategy:**
- Revert flag defaults from `true` to `false`
- Commands return to opt-in behavior
- Existing scripts work without modification
- Consider this if user feedback is strongly negative

## Open Questions

None - design is straightforward with well-understood requirements and proven patterns.
