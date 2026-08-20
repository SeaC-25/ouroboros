# Milestone 118: Workflow Smoke Entrypoint

Date: 2026-06-07

## Summary

Milestone 118 adds a one-command smoke path for the core TUI-first IDE workflow. `make smoke-workflow` now runs `scripts/smoke_workflow.sh`, which creates a fresh temporary workspace by default, initializes it, verifies strict readiness, runs the deterministic TUI snapshot path, replays the resulting run through the TUI snapshot path, and inspects the current workflow status.

The script prints a stable summary:

```text
workflow-smoke: workspace ...
workflow-smoke: logs ...
doctor: ready
run: run-...
events: ...
report: ...
replay: ok
workflow-status: status=... next_gate=...
workflow-smoke: ok
```

Full raw stdout/stderr outputs are written under `OURO_SMOKE_LOG_DIR` when provided, or a temporary log directory otherwise.

## Boundaries

This is a local smoke harness over existing CLI gates. It does not add a new app execution path, does not call providers, does not dispatch remote workers, and does not run offensive tools beyond the repository's deterministic offline default run path. It may use Go module resolution through the configured `GOPROXY` because it invokes `go run ./cmd/ouro`.

The script writes only to the provided/default workspace, the provided/default smoke log directory, and Go build/cache temp directories inherited from the environment. To avoid accidentally applying the smoke harness to a workspace with configured live targets, a provided existing non-empty `WORKSPACE` is refused unless `OURO_SMOKE_ALLOW_EXISTING_WORKSPACE=1` is explicitly set for a known-safe smoke workspace. The default path creates a fresh temporary workspace with empty scope targets.

## Implementation

- `scripts/smoke_workflow.sh`
  - Runs `ouro init`, `ouro doctor --require-safety-authorization`, `ouro run --tui`, `ouro replay --tui`, and `ouro workflow-status`.
  - Parses the run id, event count, report path, and workflow status from stable CLI lines.
  - Fails closed if strict doctor is not ready, report output is missing, core TUI panels are missing, replay is not ready, workflow status lacks a next gate, or a provided existing non-empty workspace was not explicitly opted in.

- `Makefile`
  - Adds `make smoke-workflow`.

- `release_scripts_test.go`
  - Adds coverage that the script exercises the core path, prints stable summary rows, and writes run/replay/workflow-status logs.
  - Adds coverage that existing configured workspaces are refused before invoking `go run` unless the explicit opt-in is set.
  - Updates Makefile entrypoint assertions so the new `smoke-workflow` phony target is checked without relying on one stale exact `.PHONY` line.

## Validation

Focused validation passed before this document:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run TestSmokeWorkflowScriptExercisesCoreIDEPath -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run TestSmokeWorkflowRefusesExistingConfiguredWorkspaceWithoutOptIn -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow
```

Focused validation after the hardening pass also passed:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestSmokeWorkflowRefusesExistingConfiguredWorkspaceWithoutOptIn|TestSmokeWorkflowScriptExercisesCoreIDEPath|TestReleaseMakefileAndCIEntrypoints' -count=1 -v
```

Final validation passed after documentation synchronization:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow
sh -n scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh
git diff --check
gofmt -l release_scripts_test.go
```

No release smoke is planned because release packaging and bundled manifests were not changed.
