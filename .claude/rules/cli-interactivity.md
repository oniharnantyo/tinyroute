# CLI Interactivity Rules

## Core Principle: Interactive-First (CRITICAL)

**Commands MUST NOT require typed positional arguments.** A required value is
gathered interactively (`Select` / `MultiSelect`) from live system state when it
is absent **and** a TTY is attached. Typed args remain an *optional shortcut* for
scripts, power users, and non-interactive runs — never the primary path.

**Mental model:** "args-as-escape-hatch, prompts-as-default."

- **Template implementation:** `provider add` (`internal/cli/commands.go`).
- **Primitives:** `internal/cli/interactive/` — `Select`, `MultiSelect`, `Input`,
  `Confirm`, `Password`, plus `Spinner` / `Progressbar`. Every primitive is guarded
  by `CanPrompt()` and degrades gracefully when no TTY is present.

## The Pattern

Replace the manual-arg dead-end with prompt-when-absent:

```go
// WRONG — hard-errors the moment an arg is omitted
if fs.NArg() < 1 {
    return errors.New("usage: tinyroute provider model add <provider>")
}
provName := fs.Arg(0)

// CORRECT — interactive-first
provName := fs.Arg(0) // honor the arg if the user provided it
if provName == "" {
    if !interactive.CanPrompt() {
        return fmt.Errorf("provider is required: pass it as an argument or run in a terminal")
    }
    names := providerNames(topo) // drawn from live state
    if len(names) == 0 {
        return fmt.Errorf("no providers configured in %s", svc.ConfigPath)
    }
    if len(names) == 1 {
        provName = names[0] // auto-select the single candidate
    } else {
        selected, err := interactive.Select("Select provider:", names)
        if err != nil {
            return fmt.Errorf("select provider: %w", err)
        }
        provName = selected
    }
}
```

## Rules

- **Honor args if present.** Prompt only when a required value is absent. This
  keeps the CLI backward-compatible and scriptable.
- **Non-TTY + missing arg → clear error.** Name the required value and how to
  supply it. Never silently guess a default when interactivity is unavailable.
- **Draw options from real state.** Pickers must source from system state (the
  topology's providers, a provider's `Models` whitelist, fetched catalogs), so
  selections are valid by construction. Never offer free-text `Input` where a
  bounded list exists.
- **Single candidate → auto-select.** Skip the prompt when there is exactly one
  valid option.
- **Empty source list → informational exit.** If the picker would have nothing to
  show (e.g. `model remove` on a provider with no whitelisted models), print a
  clear message and exit — do not render an empty picker.
- **Respect the interaction-control flags.** Commands that define
  `--interactive` / `--no-interactive` / `--force` MUST honor them:
  `isInteractive := interactiveFlag && !noInteractiveFlag && !forceFlag && interactive.CanPrompt()`.
- **Filter is on by default.** `pterm`'s interactive select/multiselect enables
  `Filter` with a `[type to search]` placeholder, so large catalogs (hundreds of
  models) stay usable — no custom search UI needed.

## Interaction Quality Checklist

Before marking a command interactive-first:

- [ ] Required positional args are optional; command runs with zero args in a TTY
- [ ] Missing value + TTY → prompted via `Select`/`MultiSelect` (or `Input` only
      when no bounded list exists)
- [ ] Missing value + non-TTY → clear error naming the value
- [ ] Picker options come from live state, validated by construction
- [ ] Single-candidate case auto-selects without a prompt
- [ ] Empty-option case exits cleanly with a message
- [ ] `--no-interactive` / `--force` (where defined) bypass prompts
- [ ] Errors wrapped with `fmt.Errorf("...: %w", err)` per `coding-style.md`

## Scope

Apply to the whole CLI tree over time. `provider add` is the reference; the
`provider model` group (`add` / `remove` / `list` / `test`) is the proving ground
(see OpenSpec change `add-provider-models`, Decision 5).

## References

- [coding-style.md](coding-style.md) — Go formatting, naming, error handling
- [testing.md](testing.md) — test coverage expectations
- `internal/cli/interactive/` — prompt primitives (`prompts.go`, `wizard.go`, `progress.go`)
- `internal/cli/commands.go` — `cmdProvider` / `provider add` reference handler