# Repository Guidelines

## Project Structure & Module Organization
Ouroboros is a Go CLI/TUI security IDE. The CLI entrypoint lives in `cmd/ouro/`; first-party packages live under `internal/` (`app`, `tui`, `eventlog`, `toolchain`, `report`, and related domain packages). Tool definitions and strategy data are in `toolchains/` and `configs/patterns/`. Project documentation and milestone notes are under `docs/`. The optional Electron/Vite frontend is in `gui/`, with source in `gui/src/`. Test fixtures belong in `test/fixtures/`; generated workspaces, `bin/`, `dist/`, and coverage output should stay uncommitted.

## Build, Test, and Development Commands
- `make test`: runs `GOPROXY=https://goproxy.cn,direct go test ./...`.
- `make build`: builds `bin/ouro` via `scripts/build.sh`.
- `make doctor-strict`: initializes a temporary workspace and runs strict readiness checks.
- `make release`: creates a local release archive under `dist/`.
- `make smoke-ide`: runs the local IDE smoke path with fake/offline services.
- `go run ./cmd/ouro init <workspace>`: create a workspace for manual CLI testing.
- `cd gui && npm install && npm run dev`: install and launch the GUI dev app.
- `cd gui && npm run build`: build the Vite/Electron frontend bundle.

## Coding Style & Naming Conventions
Format Go with `gofmt`; keep package names short, lowercase, and domain-oriented. Use standard Go test names such as `TestWorkflowStatusNextGate`. Keep command wiring in `cmd/ouro/` thin and put behavior in `internal/app` or a narrower package. For TypeScript/React in `gui/src/`, use PascalCase components, camelCase hooks/utilities, and Tailwind classes already present in the app.

## Testing Guidelines
Primary coverage is the Go standard test suite. Add or update `*_test.go` files beside the code being changed, especially for workflow gates, parsers, replay/eventlog behavior, and safety checks. Run `make test` before submitting; run `make doctor-strict`, `make build`, or smoke targets when changing CLI startup, workspace validation, release scripts, providers, or TUI flows. GUI changes should at least pass `cd gui && npm run build`.

## Commit & Pull Request Guidelines
Recent history uses concise prefixes such as `feat:`, `docs:`, and `merge:`; keep commit subjects imperative and specific, for example `feat: add provider readiness gate`. Pull requests should describe purpose, major files changed, validation commands run, linked issues, and screenshots or terminal output for visible TUI/GUI changes.

## Security & Configuration Tips
Use only authorized, scoped targets. Do not commit `.env*`, API keys, generated scan artifacts, or temporary workspaces. Prefer fake providers and temporary workspaces for smoke tests; document any safety-boundary bypasses and restore stricter settings before CI or production-style runs.
