# Milestone 189: Built Binary Direct-Use Smoke Entrypoint

## Summary

- Added `scripts/smoke_direct_use.sh` as a repeatable built-binary usability smoke.
- Added `make smoke-direct-use`.
- The smoke uses `bin/ouro` by default and supports an absolute `OURO_DIRECT_USE_SMOKE_BIN` override for installed-binary checks.
- The smoke runs `init`, `doctor`, `run --tui`, `replay --tui`, and `workflow-status`, then verifies the generated report plus compile-preview review and selection artifacts.
- README and development documentation now list and describe the new entrypoint.

## Safety Boundary

- This smoke proves local/offline direct binary startup usability only.
- It does not use `go run`, configure live providers, contact endpoints/targets, execute managed payload acceptance flows, download content, dispatch remote workers, or claim production completion.
- Existing non-empty workspaces are refused unless `OURO_DIRECT_USE_SMOKE_ALLOW_EXISTING_WORKSPACE=1` is explicitly set for a known-safe smoke workspace.
- True completion still depends on real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED first showed `TestReleaseMakefileAndCIEntrypoints` failed because the Makefile lacked `smoke-direct-use:`, and `TestDirectUseSmokeScriptExercisesBuiltBinaryPath` failed because `scripts/smoke_direct_use.sh` did not exist.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestReleaseMakefileAndCIEntrypoints|TestDirectUseSmokeScriptExercisesBuiltBinaryPath|TestDocumentationBashBlocksPassShellSyntaxCheck' -count=1 -v`, with root runtime about 0.047s.
- Script syntax passed with `sh -n scripts/smoke_direct_use.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh scripts/smoke_managed_payloads.sh scripts/smoke_production_acceptance.sh scripts/audit_completion.sh`.
- Real built-binary smoke passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-direct-use`; it used `/home/zss/Ouroboros/bin/ouro`, printed `doctor: ready`, produced run `run-1780989252269157529` with 69 events, replayed successfully, reported `next_gate=create-compile-preview-review-execution-dispatch`, and verified report/review/selection artifacts.
- Final hygiene passed with `gofmt -l release_scripts_test.go`, `git diff --check`, and touched-file trailing-whitespace scan returning clean.

## Remaining

- Completion remains about 99.99%, not 100%.
- The remaining gap is production evidence and final operator acceptance, not basic local IDE startup usability.
