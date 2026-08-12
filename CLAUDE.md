# tinyroute

`tinyroute` is a lightweight LLM API gateway/router written in Go. It proxies
inference requests to upstream providers, with strict `provider:model` prefix
routing, per-provider model whitelists, API-key auth, and an interactive CLI.

- **Module:** `github.com/oniharnantyo/tinyroute` (Go 1.26.4)
- **Entry point:** `main.go` → `internal/cli`
- **Key deps:** `urfave/cli/v3` (commands), `pterm` (interactive TUI), `golang.org/x/term`

## Build & Test

```bash
go build ./...        # compile
go test ./...         # run all tests
go run . serve        # run the gateway locally
```

Format before committing: `gofmt -w .`

## Project Layout

```
internal/
├── cli/            # command tree (commands.go, serve.go) + interactive/
│   └── interactive/  # pterm prompt primitives (CanPrompt-guarded)
├── config/         # topology config (providers, routes, models)
├── route/          # provider:model prefix routing + resolution
├── proxy/          # upstream request proxying
├── translate/      # provider dialect translation
├── dialect/        # dialect definitions
├── preset/         # built-in provider presets
├── auth/           # API key generation + keyfile
├── history/        # request history / blob storage
└── core/           # shared core types
```

Full design write-up: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

## Conventions

Project rules live in [`.claude/rules/`](.claude/rules/) and are loaded as
project instructions every session. Read and follow them:

- [coding-style.md](.claude/rules/coding-style.md) — Go formatting, naming, error handling, file/package organization
- [cli-interactivity.md](.claude/rules/cli-interactivity.md) — interactive-first CLI pattern (no required typed args)
- [testing.md](.claude/rules/testing.md) — 80% coverage, TDD, edge cases
- [security.md](.claude/rules/security.md) — mandatory security checks, secret management
- [performance.md](.claude/rules/performance.md) — model routing, context-window, algorithm guidance
- [karpathy-guidelines.md](.claude/rules/karpathy-guidelines.md) — think-before-code, simplicity, surgical changes

When adding a new convention, drop a file here rather than creating a parallel structure.

## Spec-Driven Workflow

This repo uses [OpenSpec](openspec/) (schema: `spec-driven`). Changes live in
`openspec/changes/<name>/` with `proposal.md` → `design.md` → `specs/` → `tasks.md`.

- Active changes: `add-provider-models`, `tinyroute-core-router`
- Inspect: `openspec list`, `openspec status --change <name>`
- The CLI is being migrated to an **interactive-first** model — see
  [cli-interactivity.md](.claude/rules/cli-interactivity.md) and the
  `add-provider-models` change (Decision 5).