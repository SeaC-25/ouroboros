# Milestone 11: Production Core Bring-Up

## Implemented
- Extended workspace initialization in `internal/app/workspace.go`.
- `ouro init <workspace>` now creates `mission.yaml`, `scope.yaml`, `providers.yaml`, artifacts, evidence, attempts, findings, reports, replay, and toolchain directories.
- Workspace initialization now writes workspace-local toolchain manifests for Web/API, Infra/Internal, and CTF/Lab packs.
- Added provider config validation and an offline provider adapter boundary.
- Added scope config validation for URL, CIDR, and challenge targets.
- Added `internal/app/run.go` with the first production run path.
- `RunWorkspace` validates provider config, scope config, storage layout, and required toolchain manifests before starting.
- `RunWorkspace` uses configured provider adapters, domain packs, toolchain manifests, SQLite event log, replay snapshot projection, TUI status projection, and Markdown report generation.
- Scope targets now produce domain-pack initial hypothesis intent events for Web/API, Infra/Internal, and CTF/Lab.
- `ouro run <workspace>` now executes the run path and prints run id, event count, and report path.
- Added `configs/providers.example.yaml`.
- Updated `toolchains/ctf-pack/manifest.yaml` to use `ctf.fast_hypothesis`, matching the CTF/Lab pack capability.
- Added app and CLI tests for init, doctor, run, domain pack hypothesis events, provider adapter rejection, invalid scope rejection, and missing storage rejection.

## Not Implemented
- Real external provider adapters are not implemented yet; the current adapter is offline and deterministic.
- Live tool execution is not implemented yet; toolchain manifests are validated and recorded as ready, but tools are not invoked.
- The run path does not start the interactive TUI program yet; it projects replay status into TUI status data for now.

## Validation
- `GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -v`
- `GOPROXY=https://goproxy.cn,direct go test ./...`
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro init /tmp/ouro-acceptance-SnYeSP/workspace`
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro doctor /tmp/ouro-acceptance-SnYeSP/workspace`
- `GOPROXY=https://goproxy.cn,direct go run ./cmd/ouro run /tmp/ouro-acceptance-SnYeSP/workspace`

## Next Step
- Continue Milestone 12 with Task 12.1: Build Script.

## Risks / Decisions
- The production run path intentionally uses an offline provider adapter so tests and acceptance commands do not require network access or live targets.
- Domain packs are exercised only through initial hypotheses from configured scope targets; live tool execution remains a later production-hardening step.
- `RunWorkspace` writes durable events first, replays them into a snapshot, then generates Markdown from the projected Evidence Graph.
