# Milestone 162: Production Acceptance Summary-Rejection Stale Stage Log Cleanup

## Summary

- Hardened `scripts/smoke_production_acceptance.sh` failure cleanup for reused production acceptance log directories.
- After the production acceptance log directory itself has been verified, failed runs remove stale regular stage logs even when `summary.txt` cannot be published.
- Added regression coverage for symlinked and directory `summary.txt` output leaves with stale `release-managed.txt`, `managed-payloads.txt`, and `workflow-closure.txt` logs.
- Existing stage-log symlink refusal, stage-log temporary publication, and ordinary stale-unpublished cleanup behavior remain covered.

## Safety Boundary

- This milestone only removes stale regular stage-log files from a verified local production acceptance log directory on failed summary-output paths.
- It does not delete symlink or non-regular stage-log outputs, execute payloads, contact providers/workers/targets, sign evidence, add append-only storage, or claim 100%.
- True completion still depends on real operator-owned evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED first showed a symlinked `summary.txt` output was rejected while stale regular stage success logs remained in the same log directory.
- Aquinas confirmed the fix shape and recommended directory summary coverage; `TestProductionAcceptanceSmokeRemovesStaleStageLogsWhenSummaryOutputDirectory` now covers that path.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmoke(RemovesStaleStageLogsWhenSummaryOutputRejected|RemovesStaleStageLogsWhenSummaryOutputDirectory|RemovesStaleUnpublishedStageLogsOnFailure|RejectsSymlinkSummaryOutputPath|RejectsSymlinkStageLogOutputPath|PreflightsStageLogOutputPaths|PublishesStageLogsThroughTempFiles)' -count=1 -v`.
- Final-evidence documentation validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocuments(StageProvenanceArtifactBindings|RequiredFields)|TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Broad production/completion regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 161s.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 361s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printing `built bin/ouro`.
- Script syntax, `gofmt -l release_scripts_test.go`, `git diff --check`, and a touched-file trailing-whitespace scan returned clean.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
