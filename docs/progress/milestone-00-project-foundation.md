# Milestone 00: Project Foundation

## Implemented
- Initialized the Go module as `github.com/ouroboros/ouroboros`.
- Added Cobra-based CLI entrypoint at `cmd/ouro/main.go`.
- Added `ouro init <workspace>` to create the real workspace layout: `mission.yaml`, `scope.yaml`, `artifacts/`, `evidence/`, `attempts/`, `findings/`, `reports/`, `replay/`, and `toolchain/`.
- Added `ouro doctor <workspace>` to validate required workspace files and directories.
- Added app metadata and banner in `internal/app`.
- Added tests for banner output, command registration, workspace initialization, and doctor validation.

## Not Implemented
- `ouro run <workspace>` only validates workspace readiness, then returns a not-implemented error for production execution.
- `ouro replay <run-id>` and `ouro toolchain` are registered but do not yet implement replay or toolchain behavior.
- Config loading, logging, event log, provider adapters, and native toolchain manifests start in later milestones.

## Validation
- `GOPROXY=https://goproxy.cn,direct go get github.com/spf13/cobra@v1.10.1`
- `GOPROXY=https://goproxy.cn,direct go test ./...`
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro init .analysis/verify-workspace`
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro doctor .analysis/verify-workspace`
- Manual workspace inspection confirmed the required files and directories were created.

## Next Step
- Start Milestone 1, Task 1.1: Goal Stack contracts in `internal/goal`.

## Risks / Decisions
- `proxy.golang.org` was unreachable from this environment, so dependency installation uses `https://goproxy.cn,direct`.
- Cobra is required; no internal command framework was introduced.
- Test doubles remain limited to tests. The product CLI path creates and validates a real workspace.
