# Milestone 159: Production Acceptance Stale Unpublished Stage Log Cleanup

## Summary

- Hardened failed `scripts/smoke_production_acceptance.sh` runs so reused log directories cannot retain stale regular stage logs for stages that were not published in the current run.
- Moved final stage log path definitions into the early verified evidence-output scope.
- Added per-stage published markers for `release-managed.txt`, `managed-payloads.txt`, and `workflow-closure.txt`.
- On failure, after the production acceptance log directory and summary output have been verified, the script removes only unpublished regular stage log files.
- Freshly published child-stage failure logs remain available for diagnostics.
- Symlink stage log paths are not removed by this cleanup path.

## Safety Boundary

- This milestone only changes stale local stage-log cleanup in the production acceptance harness.
- It does not add evidence signing, append-only storage, payload execution, downloads, package manager calls, provider/worker contact, live target contact, hidden autonomous execution, or a 100% completion claim.
- True completion still depends on real operator-owned evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED first showed a reused production acceptance log directory with old regular `release-managed.txt`, `managed-payloads.txt`, and `workflow-closure.txt` kept those stale stage logs beside a new failed `summary.txt` when the run failed before required inventory validation.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmoke(RemovesStaleUnpublishedStageLogsOnFailure|PublishesStageLogsThroughTempFiles|RejectsSymlinkStageLogOutputPath|PreflightsStageLogOutputPaths|ScriptExercisesOperatorInventoryAndPolicy)' -count=1 -v`, including root package runtime about 54s.
- Broad production/completion/final-evidence regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 113s.
- Focused documentation/static validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(DocumentationBashBlocksPassShellSyntaxCheck|FinalEvidenceRunbookDocumentsStageProvenanceArtifactBindings|ProductionAcceptanceSmoke(RemovesStaleUnpublishedStageLogsOnFailure|PublishesStageLogsThroughTempFiles|RejectsSymlinkStageLogOutputPath|PreflightsStageLogOutputPaths))' -count=1 -v`.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 361s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printing `built bin/ouro`.
- Script syntax passed with `sh -n scripts/audit_completion.sh scripts/smoke_production_acceptance.sh scripts/smoke_managed_payloads.sh scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh`.
- `gofmt -l release_scripts_test.go`, `git diff --check`, and a touched-file trailing-whitespace scan returned clean after final documentation synchronization.

## Review Notes

- A read-only sidecar review was launched for this milestone, but the agent stream disconnected with a transport timeout before returning findings.
- Local RED/GREEN, broad regression, full repository validation, strict readiness, build, and hygiene checks are the authoritative evidence for this slice.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
