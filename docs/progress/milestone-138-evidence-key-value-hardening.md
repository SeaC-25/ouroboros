# Milestone 138: Evidence Key-Value Hardening

## Implemented
- Hardened `scripts/smoke_production_acceptance.sh` so operator inventory and deployment-policy evidence files must contain only unique, valid `key=value` records before any field value is trusted.
- Hardened `scripts/audit_completion.sh` so production summary, completion attestation, and referenced inventory/policy evidence files must contain only unique, valid `key=value` records before the final audit can reach `status=ready-for-final-review`.
- Added regression coverage for duplicate production summary fields, duplicate completion attestation fields, malformed attestation lines, duplicate production inventory fields, and malformed deployment-policy lines.
- Added `Final Evidence Runbook` sections to `README.md` and `docs/development.md` with copy-paste templates for `payload-inventory.env`, `deployment-policy.env`, and `completion-attestation.env`.
- The runbook shows the `production_summary_sha256` binding command, `make smoke-production-acceptance`, `make audit-completion`, and the remaining manual `claim_100_after_real_operator_evidence_review` boundary.
- Clarified in planning records that the current usable IDE target is the explicit operator-gated workflow closure, not an ungated autonomous execution loop hidden behind the final audit.

## Not Implemented
- This milestone does not create real operator-owned production evidence or approve an attestation.
- This milestone does not make `make audit-completion` succeed without `OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY` and `OURO_COMPLETION_AUDIT_ATTESTATION`.
- This milestone does not claim 100%; the final operator evidence review remains required.

## Validation
- RED completion-audit tests first showed duplicate summary keys, duplicate attestation keys, and malformed attestation lines were ignored and still reached `completion-audit: ok`.
- RED production-acceptance tests first showed duplicate inventory fields and malformed policy lines were not rejected as evidence-shape errors.
- Focused GREEN validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmokeRejects(DuplicateInventoryFields|MalformedPolicyLines)|TestCompletionAuditScriptRejects(DuplicateProductionSummaryFields|DuplicateAttestationFields|MalformedEvidenceLines)' -count=1 -v`
- Final evidence runbook documentation validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocumentsRequiredFields' -count=1 -v`
- Broad completion/production regression passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestFinalEvidenceRunbookDocumentsRequiredFields|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`
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
  - `sh -n scripts/audit_completion.sh scripts/smoke_production_acceptance.sh scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh scripts/smoke_managed_payloads.sh`
- Diff hygiene passed:
  - `git diff --check`
- Touched-file formatting passed:
  - `gofmt -l release_scripts_test.go cmd/ouro/main.go cmd/ouro/main_test.go internal/app/planner_loop.go internal/app/provider_generation.go internal/app/provider_generation_test.go internal/app/run.go internal/app/run_test.go internal/app/toolchain.go internal/app/toolchain_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/app/worker_queue_claims_test.go internal/app/workspace.go internal/eventlog/replay.go internal/eventlog/replay_test.go internal/eventlog/sqlite.go internal/eventlog/sqlite_test.go internal/tui/app.go internal/tui/inspector.go internal/tui/replay.go internal/tui/replay_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Next Step
- Run the broad validation suite, then keep the overall IDE completion estimate just short of 100% until real operator-owned evidence and approved attestation have passed `make audit-completion` and final review.
