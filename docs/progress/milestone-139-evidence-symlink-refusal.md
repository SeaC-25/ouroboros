# Milestone 139: Evidence Symlink Refusal

## Implemented
- Hardened `scripts/smoke_production_acceptance.sh` so `OURO_PRODUCTION_ACCEPTANCE_INVENTORY` and `OURO_PRODUCTION_ACCEPTANCE_POLICY` must not be symlinks before the script parses or validates them.
- Hardened `scripts/audit_completion.sh` so `OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY`, `OURO_COMPLETION_AUDIT_ATTESTATION`, referenced inventory/policy files, and referenced stage logs must not be symlinks before validation or SHA-256 checks.
- Added regression coverage showing the previous behavior followed symlinked inventory, production summary, and stage log paths.
- Updated the final evidence runbook in `README.md` and `docs/development.md` to state that evidence files must be regular files and must not be symlinks.

## Not Implemented
- This does not create real operator-owned production evidence.
- This does not make symlinked payload inputs a trust boundary in itself; managed payload import/update gates already retain their own source/path checks.
- This does not claim 100%; the final operator evidence review remains required.

## Validation
- RED symlink tests first showed current scripts followed symlinked evidence:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmokeRejectsSymlinkInventoryFile|TestCompletionAuditScriptRejectsSymlink(ProductionSummary|StageLog)' -count=1 -v`
- Focused GREEN symlink validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmokeRejectsSymlinkInventoryFile|TestCompletionAuditScriptRejectsSymlink(ProductionSummary|StageLog)' -count=1 -v`
- Final evidence runbook documentation validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocumentsRequiredFields' -count=1 -v`
- Broad production/completion regression passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestFinalEvidenceRunbookDocumentsRequiredFields|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`
- Strict readiness passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
- Build validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- Script syntax passed:
  - `sh -n scripts/audit_completion.sh scripts/smoke_production_acceptance.sh scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh scripts/smoke_managed_payloads.sh`
- Diff hygiene passed:
  - `git diff --check`
- Touched-file formatting passed:
  - `gofmt -l release_scripts_test.go`

## Next Step
- Run broad production/completion regression and repository hygiene checks. Keep the overall IDE target just short of 100% until real operator-owned evidence and approved attestation pass the final audit and manual review.
