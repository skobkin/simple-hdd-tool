# Repository Guidelines

## Project Structure & Module Organization
`cmd/simple-hdd-tool/main.go` is the CLI entrypoint. Core code lives under `internal/`:

- `internal/app`: Bubble Tea application state, config, and views.
- `internal/linux`: Linux-specific disk discovery, `/proc` parsing, read-load, and device removal.
- `internal/domain`: shared domain types and health logic.
- `internal/format`: formatting helpers for sizes, durations, and display values.
- `internal/buildinfo`: version injection for release builds.

Repository docs live in `README.md` and `PRD.md`. Build artifacts go to `build/` and should not contain hand-edited source.

## Build, Test, and Development Commands
- `go test ./...`: run the full test suite across all packages.
- `go build ./cmd/simple-hdd-tool`: compile the local development binary.
- `mkdir -p build && go build -o ./build/simple-hdd-tool ./cmd/simple-hdd-tool`: produce the standard binary used in the README.
- `CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -X github.com/skobkin/simple-hdd-tool/internal/buildinfo.Version=dev' -o ./build/simple-hdd-tool-static ./cmd/simple-hdd-tool`: create a stripped static build.
- `golangci-lint run`: run the configured linters and format checks from `.golangci.yml`.

## Coding Style & Naming Conventions
Use standard Go formatting with tabs and keep code `gofmt`/`goimports` clean. Package names should stay short and lowercase; exported identifiers use `CamelCase`, unexported ones use `camelCase`. Keep Linux-specific behavior inside `internal/linux` and avoid leaking it into generic domain or formatting packages. Prefer table-driven tests for parser and state logic.

## Testing Guidelines
Tests use Go’s built-in `testing` package. Keep tests next to the code they cover and name files `*_test.go`; use `*_more_test.go` only when splitting a large package test suite. Run `go test ./...` before opening a PR. Add coverage for new disk classification rules, `/proc` parsing branches, and UI model behavior when touching those areas.

## Commit & Pull Request Guidelines
Recent history follows Conventional Commit-style prefixes such as `feat(ui):`, `build(ci):`, `test:`, and `docs:`. Keep commit subjects imperative and scoped when useful. PRs should include a short description, linked issue if applicable, and terminal screenshots or text samples for TUI-visible changes. Call out Linux, root-only, or `smartctl`-dependent behavior explicitly so reviewers can validate it.

## Definition of Done
- Before considering any task as completed, run checks:
  - `go vet`
  - `gofmt`
  - `go test`
  - `golangci-lint`
- Make sure that if any documented behavior was changed, documentation was updated too