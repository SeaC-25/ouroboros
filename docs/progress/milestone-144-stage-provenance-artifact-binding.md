# Milestone 144: Stage Provenance Artifact Binding

## Implemented
- Added trusted shell/path metadata, production/stage script paths, and production/stage script SHA-256 fields to accepted production summaries.
- Hardened completion audit so it rehashes the recorded production acceptance, release-managed, managed-payloads, and workflow-closure scripts before accepting the summary.
- Hardened completion audit so it requires the referenced stage workspaces to exist and contain key stage artifacts: release-managed rule link, managed binary payload import, and workflow report-selection marker.
- Updated final evidence documentation and tests to describe the new stage provenance and workspace artifact binding.

## Not Implemented
- This does not cryptographically sign the production summary or move evidence into an append-only artifact channel.
- This does not remove the need for final human/operator evidence review.

## Validation
- RED tests first showed completion audit accepted internally consistent stage logs without any stage workspace artifacts, and production summaries lacked trusted shell/path plus stage script digest fields.
- Focused GREEN validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(ProductionAcceptanceSmokeScriptExercisesOperatorInventoryAndPolicy|CompletionAuditScriptRejectsMissingStageWorkspaceArtifacts|CompletionAuditScriptValidatesProductionSummaryAndAttestation)' -count=1 -v`
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
