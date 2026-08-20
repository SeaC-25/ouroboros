# Milestone 140: Evidence Absolute Path Enforcement

## Implemented
- Hardened `scripts/smoke_production_acceptance.sh` so `OURO_PRODUCTION_ACCEPTANCE_INVENTORY`, `OURO_PRODUCTION_ACCEPTANCE_POLICY`, and `OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR` must be absolute paths.
- Hardened `scripts/audit_completion.sh` so `OURO_COMPLETION_AUDIT_LOG_DIR`, `OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY`, `OURO_COMPLETION_AUDIT_ATTESTATION`, referenced inventory/policy files, and referenced stage logs must be absolute paths.
- Added regression coverage showing the previous behavior accepted relative production acceptance inventory paths, relative completion summary paths, and relative stage log paths.
- Updated `README.md` and `docs/development.md` final evidence runbooks to require absolute paths for evidence files and audit log directories.

## Not Implemented
- This does not create real operator-owned production evidence or an approved attestation.
- This does not make the final completion gate claim 100%; it remains `ready-for-final-review` until real evidence is reviewed.

## Validation
- RED relative-path tests first showed current scripts accepted or followed relative evidence paths:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmokeRejectsRelativeEvidencePaths|TestCompletionAuditScriptRejectsRelative(ProductionSummaryPath|StageLogPath)' -count=1 -v`
- Focused GREEN relative-path validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmokeRejectsRelativeEvidencePaths|TestCompletionAuditScriptRejectsRelative(ProductionSummaryPath|StageLogPath)' -count=1 -v`
- Runbook documentation validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocumentsRequiredFields' -count=1 -v`
- Broad production/completion regression passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestFinalEvidenceRunbookDocumentsRequiredFields|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`
- Hygiene checks returned clean:
  - `sh -n scripts/audit_completion.sh scripts/smoke_production_acceptance.sh scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh scripts/smoke_managed_payloads.sh`
  - `git diff --check`
  - `gofmt -l release_scripts_test.go`

## Next Step
- Keep the full usable IDE target just short of 100% until real operator-owned evidence and approved attestation pass the final audit and manual review.
