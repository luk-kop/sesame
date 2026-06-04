# Repository Guidelines

## Project Structure & Module Organization

SeSaMe is a Go CLI/TUI for AWS SSM-enabled EC2 access. The entry point is `cmd/sesame/main.go`. Core packages live under `internal/`: `cli`, `app`, `awsclient`, `session`, `health`, `domain`, and `tui`. Reusable TUI widgets are in `internal/tui/components`, screen helpers in `internal/tui/views`, and docs in `docs/`. Tests sit next to package code as `*_test.go`.

## Build, Test, and Development Commands

- `make test`: runs `go test ./...` with the repository-local Go build cache.
- `make fmt`: runs `gofmt -w cmd internal`.
- `make build`: builds `bin/sesame` with version metadata.
- `make build-release`: cross-compiles Linux amd64/arm64 archives and checksums.
- `make run`: runs `go run ./cmd/sesame --help`.
- `make tidy`: syncs `go.mod` and `go.sum`.
- `make clean`: removes `bin/` and `.cache/`.

Prefer Make targets so local cache and linker settings stay consistent.

## Coding Style & Naming Conventions

Use idiomatic Go and keep files `gofmt` formatted. Package names are lowercase and concise. Document exported identifiers when they form a package API; keep most implementation details unexported. Keep JSON tags on `internal/domain` types stable because CLI JSON output is a scripting contract. Follow existing TUI patterns before adding abstractions.

## Testing Guidelines

Use Go's standard `testing` package. Name tests `Test<Behavior>` and keep focused scenario tests close to the changed package. Use hand-written fakes through `internal/app` interfaces; do not make AWS calls. Add or update tests for CLI/TUI behavior, AWS mapping, filtering, and session state changes. Run `make test` before handoff; run `make fmt` after editing Go files.

## Commit & Pull Request Guidelines

Recent history uses Conventional Commit-style subjects such as `feat: add K9s-style table controls to TUI`, `fix: keep header visible and improve instance details`, `docs: update README`, and `chore: align local dev tooling`. Pre-commit hooks may enforce linting, tests, module tidiness, vulnerability checks, and commit message format. PRs should explain the change, mention affected commands or screens, link issues, and include test results.

## Security & Configuration Tips

Do not commit AWS credentials, account-specific secrets, or generated release artifacts. Region must be resolved explicitly; do not silently assume a default. Profile and region switching must not mutate AWS config files or expose secret values. Preserve documented exit codes and the `sesame list --output json` contract when changing error paths or domain types.
