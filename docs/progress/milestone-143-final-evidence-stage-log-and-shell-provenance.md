# Milestone 143: Final Evidence Stage Log And Shell Provenance

## Implemented
- Hardened `scripts/audit_completion.sh` so digest-matched stage logs must also contain the expected success sentinels: `release-managed-smoke: ok`, `managed-payloads-smoke: ok`, and `workflow-closure-smoke: ok`.
- Hardened `scripts/audit_completion.sh` and `scripts/smoke_production_acceptance.sh` so both final evidence gates start through `/bin/sh` and reset `PATH` before any external prelude command, parser command, digest command, or marker check.
- Hardened `scripts/smoke_production_acceptance.sh` so child acceptance stages are invoked through a trusted system shell (`/bin/sh` or `/usr/bin/sh`) and a trusted `PATH`, instead of caller-controlled `PATH/sh`.
- Updated final evidence runbooks so shell code blocks remain copy-paste-safe and production acceptance documentation reflects the trusted child-stage boundary.

## Not Implemented
- This does not create real operator-owned production evidence or an approved completion attestation.
- This does not turn the completion audit into an automatic 100% claim; it still writes `ready-for-final-review`.
- This does not replace final operator review of the production summary, stage logs, inventory, policy, payloads, and attestation.

## Validation
- RED tests first showed completion audit accepted digest-matched logs without semantic success markers, runbook prose could appear inside `bash` blocks, `smoke_production_acceptance.sh` could invoke a fake caller-controlled `PATH/sh`, direct script execution could be intercepted by fake `PATH/sh`, production acceptance prelude could call fake `dirname`, and completion audit digest checks could be spoofed by fake `sha256sum`.
- Focused GREEN validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbook(BashBlocksAreCopyPasteSafe|DocumentsRequiredFields)|TestCompletionAuditScriptRejectsStageLogWithoutOKSentinel|TestProductionAcceptanceSmokeUsesTrustedShellForChildStages' -count=1 -v`
- Entry/PATH provenance validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(ProductionAcceptanceSmokeDirectExecUsesTrustedShellAndPathBeforePrelude|CompletionAuditScript(UsesTrustedPathForDigestChecks|DirectExecUsesTrustedShellBeforePrelude))' -count=1 -v`
- Broad production/completion regression passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbookDocumentsRequiredFields|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`
- Full repository validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- Hygiene checks returned clean:
  - `sh -n scripts/audit_completion.sh scripts/smoke_production_acceptance.sh scripts/smoke_managed_payloads.sh scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh`
  - `gofmt -l release_scripts_test.go`
  - `git diff --check`

## Next Step
- Keep the full usable IDE target just short of 100% until real operator-owned evidence and approved attestation pass the final audit and manual review.
