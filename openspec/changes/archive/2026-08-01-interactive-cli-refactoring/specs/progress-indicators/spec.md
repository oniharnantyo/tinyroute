# Progress Indicators Specification

## ADDED Requirements

### Requirement: Spinner for indeterminate progress
The system SHALL display a spinner animation for operations with unknown duration by default.

#### Scenario: Provider testing with spinner (default behavior)
- **WHEN** user runs `tinyroute provider test` without any flags
- **THEN** system displays spinner while testing each provider
- **AND** spinner shows current provider being tested
- **AND** spinner updates as testing progresses through providers

#### Scenario: Skip spinner with --no-interactive flag
- **WHEN** user runs `tinyroute provider test --no-interactive`
- **THEN** system executes tests without displaying spinner

### Requirement: Progress bar for determinate progress
The system SHALL display a progress bar for operations with known quantity by default.

#### Scenario: Blob compaction with progress bar (default behavior)
- **WHEN** user runs `tinyroute compact` without any flags
- **THEN** system displays progress bar showing compaction progress
- **AND** progress bar shows current position and total count
- **AND** progress bar updates as each blob is processed

#### Scenario: Skip progress bar with --no-interactive flag
- **WHEN** user runs `tinyroute compact --no-interactive`
- **THEN** system executes compaction without displaying progress bar

#### Scenario: Dry-run compaction shows status
- **WHEN** user runs `tinyroute compact --dry-run` without flags
- **THEN** system displays progress bar for analysis phase
- **AND** system reports how many blobs would be removed

### Requirement: Progress message updates
The system SHALL update progress indicators with contextual messages during operation.

#### Scenario: Provider testing progress updates
- **WHEN** spinner is active during provider testing
- **THEN** system updates spinner text to show current provider
- **AND** system displays "Testing anthropic..." then "Testing openai..." etc.

#### Scenario: Operation completion message
- **WHEN** operation completes successfully
- **THEN** system displays completion message with summary
- **AND** system shows "Tested 3 providers, all healthy" or similar

### Requirement: Error handling with progress indicators
The system SHALL handle errors gracefully while progress indicators are active.

#### Scenario: Error during operation with spinner
- **WHEN** error occurs during operation with active spinner
- **THEN** system stops spinner and displays error message
- **AND** system shows which operation failed and why

#### Scenario: Error during operation with progress bar
- **WHEN** error occurs during operation with active progress bar
- **THEN** system stops progress bar and displays error message
- **AND** system shows progress count at time of failure

### Requirement: Progress indicator terminal compatibility
The system SHALL only display progress indicators when running in a terminal environment, with automatic degradation for non-terminals.

#### Scenario: Non-terminal environment
- **WHEN** operation runs in non-terminal context (piped, CI/CD)
- **THEN** system automatically skips progress indicators
- **AND** system displays simple text status messages instead
- **AND** this automatic override applies regardless of default interactive behavior

#### Scenario: Terminal with --no-interactive flag
- **WHEN** operation runs with `--no-interactive` flag
- **THEN** system skips progress indicators
- **AND** operation completes without visual progress feedback

#### Scenario: Terminal with --force flag
- **WHEN** operation runs with `--force` flag
- **THEN** system skips progress indicators (alias for --no-interactive)
- **AND** operation completes without visual progress feedback

### Requirement: Interruptible operations with progress indicators
The system SHALL allow users to interrupt operations while progress indicators are active.

#### Scenario: User interrupts during spinner operation
- **WHEN** user presses Ctrl+C while spinner is active
- **THEN** system stops spinner and displays cancellation message
- **AND** system exits cleanly without partial state corruption

#### Scenario: User interrupts during progress bar operation
- **WHEN** user presses Ctrl+C while progress bar is active
- **THEN** system stops progress bar and displays cancellation message
- **AND** system reports how many items were processed before interruption

### Requirement: Progress indicator cleanup on completion
The system SHALL ensure progress indicators are properly removed/stopped on operation completion.

#### Scenario: Successful operation completion
- **WHEN** operation completes successfully
- **THEN** system stops spinner or progress bar cleanly
- **AND** terminal state is restored to normal operation

#### Scenario: Operation failure
- **WHEN** operation fails with error
- **THEN** system stops spinner or progress bar cleanly
- **AND** error message is displayed without progress indicator artifacts
