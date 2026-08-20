# Milestone 137: Completion Audit

## Implemented
- Added `scripts/audit_completion.sh` and `make audit-completion`.
- The completion audit requires `OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY` and `OURO_COMPLETION_AUDIT_ATTESTATION`.
- Both evidence files are parsed as fixed `key=value` data and are not sourced as shell.
- The production summary must be an accepted operator-owned production acceptance summary with passed release-managed, managed-payload, and workflow-closure stages, explicit safe boundary booleans including `no_live_targets=true`, stage log paths, and stage log SHA-256 fields that match the referenced files.
- The audit revalidates the referenced inventory and policy files for the production acceptance schema and required operator-owned fields.
- The attestation must be approved and explicitly record reviewer identity/timestamp, `production_summary_sha256` bound to the referenced summary, `production_evidence=operator_owned`, `fixture_evidence_allowed=false`, and reviewed scope, safety, release-managed, managed-payload, workflow-closure, and documentation fields.
- Successful audits write `completion-audit.txt` with `status=ready-for-final-review`, SHA-256 digests for the production summary and attestation, stage acceptance status, stage log digests, and `remaining_manual_step=claim_100_after_real_operator_evidence_review`.
- Failed audits write a failed `completion-audit.txt` when possible and fail closed before printing an ok status.

## Not Implemented
- The audit does not execute payloads, run tools, mutate workspaces, download content, call package managers, call providers, contact workers, or contact endpoints/targets.
- The audit does not turn fixture evidence into production evidence.
- The audit does not claim 100% by itself. It records `ready-for-final-review`; a real operator evidence review is still required before declaring the overall IDE target complete.

## Validation
- Focused completion audit validation passed after RED tests first showed missing stage-log digest verification, missing `no_live_targets` enforcement, and missing attestation-to-summary binding:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`
- Production acceptance plus completion audit root regression passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`
- Full repository validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- Strict readiness passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
- Build validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- Core workflow smoke passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow`
- Workflow closure smoke passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow-closure`
- Release-managed smoke passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-release-managed`
- Script syntax passed:
  - `sh -n scripts/audit_completion.sh scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh scripts/smoke_managed_payloads.sh scripts/smoke_production_acceptance.sh`
- Diff hygiene passed:
  - `git diff --check`
- Touched-file formatting passed:
  - `gofmt -l release_scripts_test.go cmd/ouro/main.go cmd/ouro/main_test.go internal/app/planner_loop.go internal/app/provider_generation.go internal/app/provider_generation_test.go internal/app/run.go internal/app/run_test.go internal/app/toolchain.go internal/app/toolchain_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/app/worker_queue_claims_test.go internal/app/workspace.go internal/eventlog/replay.go internal/eventlog/replay_test.go internal/eventlog/sqlite.go internal/eventlog/sqlite_test.go internal/tui/app.go internal/tui/inspector.go internal/tui/replay.go internal/tui/replay_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Next Step
- Run the completion audit against real operator-owned production evidence and an approved attestation before claiming 100%.

## Risks / Decisions
- `make audit-completion` intentionally fails without real evidence inputs.
- The line-oriented audit result keeps the shell-only acceptance path dependency-light, but the final claim still depends on human/operator review of the referenced evidence.
- Generated fixture tests prove the gate mechanics, stage-log digest matching, no-live-target enforcement, attestation binding, and fail-closed fixture rejection, not the existence of real operator production evidence.
