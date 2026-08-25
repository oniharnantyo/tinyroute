## 1. Router: `combo:` key form

- [x] 1.1 Write failing router tests: `combo:<name>` resolves identically to the bare name; `combo:<name>:<model>` composes with `$model` substitution; `combo:<unknown>` errors naming the identifier
- [x] 1.2 Write failing fallthrough tests: a provider named `combo` serves `combo:<model>` when no combo shadows it; a declared combo takes precedence over the provider
- [x] 1.3 Implement the prefix-strip re-entry in `Resolve` (`internal/route/router.go`) per design decision 1-2; make tests pass
- [x] 1.4 Update `Router.Models()` to list combos in `combo:<name>` form only; assert in `router_test.go` that every listed combo id resolves on the surface

## 2. Combo name guard

- [x] 2.1 Write failing test in `internal/config/combos_test.go`: `ValidateComboName` rejects the name `combo`
- [x] 2.2 Add the guard to `ValidateComboName` (`internal/config/combos.go`); make test pass

## 3. Discovery emits key form

- [x] 3.1 Update `internal/clients/installer_models_test.go`: `DiscoverModelsForDialect` returns `combo:<name>` per declared combo (supersedes the uncommitted bare-name append); no bare combo names in output
- [x] 3.2 Update `DiscoverModelsForDialect` (`internal/clients/installer.go`) to emit the key form; make tests pass

## 4. CLI install picker

- [x] 4.1 Verify `internal/cli/clients.go` install flow offers `combo:<name>` entries unchanged (options flow from discovery; value == display) and `--model combo:<name>` is honored; add a test in `internal/cli/clients_integration_test.go` covering the combo entry path end-to-end

## 5. Dashboard picker grouping

- [x] 5.1 Write failing test in `internal/dashboard/clients_test.go`: client detail picker groups all `combo:*` entries under a single "Combos" header; no combo renders under "defaults"; selected combo writes the key form
- [x] 5.2 Update `groupModelsByProvider` (`internal/dashboard/view_client_detail.go`) to label the `combo` group "Combos"; make tests pass

## 6. Verification

- [x] 6.1 Run `go test ./...` — all packages pass; coverage on touched packages ≥ 80%
- [x] 6.2 Run `gofmt -w .` and confirm a clean diff
- [x] 6.3 Run `openspec validate add-combo-prefix-key` — valid
