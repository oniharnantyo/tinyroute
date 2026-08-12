## 1. Setup & Dependencies

- [x] 1.1 Add `github.com/pterm/pterm` dependency to go.mod
- [x] 1.2 Create `internal/cli/interactive/` package directory structure
- [x] 1.3 Run `go mod tidy` to sync dependencies

## 2. Interactive Package Foundation

- [x] 2.1 Implement `CanPrompt()` terminal detection function in prompts.go
- [x] 2.2 Create error handling and interrupt signal handling for interactive operations
- [x] 2.3 Write unit tests for terminal detection (mock isTerminal for testing)

## 3. Interactive Prompts Implementation

- [x] 3.1 Implement `Confirm(message, default) bool` using PTerm InteractiveConfirm
- [x] 3.2 Implement `Password(message) string` using PTerm InteractiveTextInput.WithMask("*")
- [x] 3.3 Implement `Input(message, default, validator) string` using PTerm InteractiveTextInput
- [x] 3.4 Implement `Select(message, options) string` using PTerm InteractiveSelect
- [x] 3.5 Add graceful degradation for all prompt functions when CanPrompt() returns false
- [x] 3.6 Write unit tests for all prompt functions (use bufio/buffers for automation)

## 4. Progress Indicators Implementation

- [x] 4.1 Implement `StartSpinner(message)` using PTerm DefaultSpinner
- [x] 4.2 Implement `StartProgressbar(total)` using PTerm DefaultProgressbar
- [x] 4.3 Implement `Update(current, total)` for both spinner and progress bar
- [x] 4.4 Implement `Stop()` to cleanly terminate progress indicators
- [x] 4.5 Add graceful degradation when CanPrompt() returns false
- [x] 4.6 Write unit tests for progress functions

## 5. Interactive Wizard Implementation

- [x] 5.1 Create wizard framework with step navigation in wizard.go
- [x] 5.2 Implement welcome message and overview step
- [x] 5.3 Implement provider selection step with preset list
- [x] 5.4 Implement credential collection step for selected providers
- [x] 5.5 Implement API key creation step with key generation
- [x] 5.6 Implement configuration validation and summary step
- [x] 5.7 Add back/forward navigation between wizard steps
- [x] 5.8 Implement wizard cancellation and cleanup handling
- [x] 5.9 Write unit tests for wizard flow and navigation

## 6. Command Integration (BREAKING CHANGE - MUST REDO)

- [x] 6.1 Change ALL `--interactive` flag defaults from `false` to `true` (7 commands)
- [x] 6.2 Add `--no-interactive` flag to all interactive commands
- [x] 6.3 Update conditional logic: `if *interactiveFlag && !*forceFlag && !*noInteractiveFlag`
- [x] 6.4 Modify `cmdKeysRevoke` with new default interactive behavior
- [x] 6.5 Modify `cmdAuthSet` with new default interactive behavior
- [x] 6.6 Modify `cmdKeysCreate` with new default interactive behavior
- [x] 6.7 Modify `cmdAdd` with new default interactive behavior
- [x] 6.8 Modify `cmdTest` with new default interactive behavior
- [x] 6.9 Modify `cmdCompact` with new default interactive behavior
- [x] 6.10 Modify `cmdInit` with new default interactive behavior
- [x] 6.11 Update cli.go global flags (default to true)
- [x] 6.12 Update cli.go help text to reflect new default behavior

## 7. Testing (UPDATED for default interactive)

- [x] 7.1 Test default behavior: commands prompt WITHOUT any flags
- [x] 7.2 Test `--no-interactive` flag disables prompts
- [x] 7.3 Test `--force` flag works as alias
- [x] 7.4 Test piped commands auto-degrade to non-interactive
- [x] 7.5 Write integration tests for new default behavior
- [x] 7.6 Test script compatibility with `--no-interactive`
- [x] 7.7 Verify CI/CD environments don't hang (auto non-interactive)

## 8. Documentation (UPDATED for breaking change)

- [x] 8.1 Update README: announce breaking change clearly
- [x] 8.2 Add migration guide for script authors
- [x] 8.3 Update all examples to show new default behavior
- [x] 8.4 Document `--no-interactive` flag usage
- [x] 8.5 Update CHANGELOG with BREAKING CHANGE notice
- [x] 8.6 Add deprecation timeline if using gradual migration

## 9. Verification & Cleanup (UPDATED)

- [x] 9.1 Run full test suite: `go test ./...`
- [x] 9.2 Manual testing: Run commands without flags (should prompt)
- [x] 9.3 Manual testing: Run commands with `--no-interactive` (should not prompt)
- [x] 9.4 Manual testing: Run commands with `--force` (should not prompt)
- [x] 9.5 Manual testing: Test piped commands (should not hang)
- [x] 9.6 Manual testing: Test existing scripts break (expected)
- [x] 9.7 Run `gofmt` on all modified files
- [x] 9.8 Verify scripts work with `--no-interactive` added
