## 1. OAuth Signal Handling

- [x] 1.1 Update `cmdAuthLogin`, `runPKCEFlow`, and `runDeviceCodeFlow` in `internal/cli/auth.go` to wrap context with `signal.NotifyContext` for SIGINT/SIGTERM.
- [x] 1.2 Ensure `runPKCEFlow` and `runDeviceCodeFlow` exit cleanly when context is cancelled, without calling `store.Save`.

## 2. Deferred Config Persistence in Provider Add

- [x] 2.1 Refactor `cmdAdd` in `internal/cli/commands.go` so `config.WriteTopology` is not invoked before `cmdAuthLogin` when immediate OAuth login is confirmed.
- [x] 2.2 If `cmdAuthLogin` fails or is cancelled during `cmdAdd`, ensure topology changes are discarded, an error notice is displayed, and `config.json` is not updated.
- [x] 2.3 Ensure explicit user declination of immediate OAuth login ("Log in now? [N]") correctly writes unauthenticated provider topology to `config.json`.

## 3. Interactive Prompt Error Handling Across Commands

- [x] 3.1 Audit and update prompt error checking in `cmdAdd`, `cmdAuthSet`, and `cmdAuthImport` to abort without mutating disk when prompts return an error (e.g. SIGINT).
- [x] 3.2 Audit `RunInitWizard` in `internal/cli/interactive/wizard.go` and `cmdAgentInstall` in `internal/cli/agent.go` to ensure prompt errors abort without persisting configuration.

## 4. Verification and Tests

- [x] 4.1 Add test in `internal/cli/auth_test.go` verifying that cancelled/failed OAuth login during `provider add` leaves `config.json` unchanged.
- [x] 4.2 Run `go test ./...` and `openspec validate --change prevent-unauthenticated-config-persist` to ensure all tests and specs pass.
