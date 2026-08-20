# Milestone 135: Workflow Closure Smoke

## Implemented
- Added `scripts/smoke_workflow_closure.sh` and `make smoke-workflow-closure`.
- The smoke creates a fresh workspace by default and refuses an existing non-empty supplied workspace unless `OURO_WORKFLOW_CLOSURE_SMOKE_ALLOW_EXISTING_WORKSPACE=1` is set.
- The smoke runs strict doctor, `run --defer-report-projection`, workflow-status gate checks, compile-preview review execution dispatch creation, explicit compile dispatch execution, report projection dispatch creation, explicit report dispatch execution, accepted report execution annotation, current report execution selection, selection readback, and closed-run `replay --tui`.
- The script writes raw logs under `OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR` or a temporary log directory.
- Release script regression tests cover the Makefile entrypoint, full paths-with-spaces closure smoke, expected final artifacts, final report selection marker, replay snapshot, and refusal before invoking `go` for existing non-empty workspaces.

## Not Implemented
- No provider-preview fixture, provider generation, provider routing, or provider usage ledger is part of this smoke.
- No managed payload import, release-managed install, managed update download/apply, package-manager call, remote download, or payload acceptance path is part of this smoke.
- The script does not evaluate human-readable `workflow-command` hints because those are not shell-quoted for workspace or artifact paths with spaces.
- The script does not add new autonomous runner behavior. It exercises only existing explicit operator gates and the reviewed compile execution path.

## Validation
- RED first failed because `smoke-workflow-closure` and `scripts/smoke_workflow_closure.sh` did not exist:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestSmokeWorkflowClosure(ScriptExercisesOperatorGateClosure|RefusesExistingConfiguredWorkspaceWithoutOptIn)|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`
- GREEN focused validation passed after adding the script, Make target, and regression coverage with the same command.
- Direct closure smoke passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow-closure` ended with `workflow-closure-smoke: ok`.
- Full repository validation passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`.
- Strict readiness passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printed `workspace: ready`.
- Build validation passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printed `built bin/ouro`.
- Core workflow smoke passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow` ended with `workflow-smoke: ok`.
- Release-managed smoke passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-release-managed` ended with `release-managed-smoke: ok`.
- Script syntax, diff hygiene, and touched-file gofmt passed with `sh -n scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh scripts/smoke_managed_payloads.sh`, `git diff --check`, and `gofmt -l release_scripts_test.go`.

## Risks / Decisions
- `workflow-command` remains a human navigation hint, not a script API. The smoke asserts `next_gate` values, then uses workspace artifact discovery and command `artifact:` lines for exact paths.
- `run --defer-report-projection` is intentional because it leaves report projection as explicit operator gates. The normal `make smoke-workflow` entrypoint remains the core `run --tui` / `replay --tui` proof.
