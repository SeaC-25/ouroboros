# Repository Guidelines

## Project Structure & Module Organization
This repository is now a Go implementation workspace for Ouroboros, a TUI-first automated pentest compiler/IDE. The maintained root artifacts are:

- `docs/`: platform architecture, implementation plans, and milestone progress documents.
- `cmd/ouro/`: CLI entrypoint for workspace initialization, doctor checks, run orchestration, replay, and toolchain commands.
- `internal/`: first-party Go packages for app orchestration, state, compiler, domain packs, toolchain runtime, TUI, replay, and reports.
- `toolchains/`: bundled native toolchain manifests.
- `configs/patterns/`: built-in strategy pattern YAML files.
- `model/`: upstream reference source archives. Treat these zip files as immutable inputs.
- `.analysis/`: extracted scratch workspace used for static review of the archives. Regenerate it when needed; do not treat it as canonical source.
- `task_plan.md`, `findings.md`, `progress.md`: analysis state and decision logs.

Keep generated analysis output under `.analysis/`, not in application packages.
After each milestone, write `docs/progress/milestone-XX-<slug>.md` before marking the phase complete.

## Build, Test, and Development Commands
Useful repository commands are:

- `rg --files`: inventory tracked working files quickly.
- `make test`: run `GOPROXY=https://goproxy.cn,direct go test ./...`.
- `make build`: build `bin/ouro` through `scripts/build.sh`.
- `make release`: build a local release archive under `dist/` and write `SHA256SUMS`.
- `make doctor`: initialize a temporary workspace and run `ouro doctor`.
- `make doctor-strict`: initialize a temporary workspace and run `ouro doctor --require-safety-authorization`.
- `unzip -l model/<archive>.zip | less`: inspect an archive without extracting it.
- `mkdir -p .analysis/src && unzip -q model/<archive>.zip -d .analysis/src`: refresh a local extraction for review.
- `GOPROXY=https://goproxy.cn,direct go test ./...`: run the Go test suite with the mirror used in this environment.
- `GOPROXY=https://goproxy.cn,direct VERSION=0.1.0-dev ./scripts/release.sh`: build a single-target local release package.
- `PREFIX="$HOME/.local" ./scripts/install.sh dist/<archive>.tar.gz`: install from a local release archive without network fetches.
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro init <workspace>`: create a production workspace layout.
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro doctor <workspace>`: validate workspace readiness.
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro doctor <workspace> --require-safety-authorization`: fail readiness when disabled safety boundaries lack matching authorization metadata.
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro run <workspace>`: execute the offline production run path and write durable events plus a Markdown report.
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro run <workspace> --tui`: execute the run path and print a deterministic TUI snapshot.
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro run <workspace> --interactive`: execute the run path and launch the replay-derived Bubble Tea TUI.
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro replay <workspace> <run-id>`: replay a recorded run summary from the workspace event log.
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro replay <workspace> <run-id> --tui`: replay a recorded run and print the TUI snapshot projection.
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro replay <workspace> <run-id> --interactive`: replay a recorded run and launch the replay-derived Bubble Tea TUI.
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro toolchain list <workspace>`: inspect workspace-local toolchain manifests.
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro toolchain health <workspace>`: inspect built-in toolchain readiness.

Run project-specific checks only inside extracted references when validating claims, for example:

- `cd .analysis/src/Kali-Security-MCP-main && pytest`
- `cd .analysis/src/BreachWeave-main && bun run typecheck`
- `cd .analysis/src/xalgorix-main && make test`

## Coding Style & Naming Conventions
For Markdown, use clear headings, short paragraphs, and concrete examples. Keep architecture documents in `docs/`; use date-prefixed filenames for plans under `docs/superpowers/plans/`. Preserve existing Chinese technical terminology in `docs/Ouroboros_平台架构设计书.md` unless intentionally revising it. For future code, follow the local toolchain conventions of the language in use: `gofmt` for Go, 4-space indentation for Python, and TypeScript type checks for Bun packages.

Add new Domain Packs under `internal/domain/<name>/` with `pack.go`, `hypotheses.go`, and `pack_test.go`. Add matching tool manifests under `toolchains/<pack>/manifest.yaml`, strategy patterns under `configs/patterns/`, and TUI panels under `internal/tui/`. See `docs/development.md` for the current extension map.

## Testing Guidelines
First-party Go tests use standard `*_test.go` files and run with `go test ./...`. Existing reference projects use mixed conventions: Python tests follow `tests/test_*.py`, Go tests use standard `go test ./...`, and BreachWeave currently exposes type checking through Bun.

## Commit & Pull Request Guidelines
This workspace is not currently a Git repository, so no local commit history is available. Use short, imperative commit messages such as `docs: update Ouroboros runtime plan` once version control is initialized. Pull requests should include the purpose, changed files, validation commands run, and screenshots only when UI behavior changes.

## Security & Configuration Tips
Do not run offensive tools against live targets from this workspace. Keep analysis static unless explicit authorization and scope are documented elsewhere. Do not commit secrets, API keys, extracted credentials, or generated scan artifacts.
