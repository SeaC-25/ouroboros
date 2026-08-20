# Milestone 158: Production Acceptance Stage Log Temporary Publication

## Summary

- Hardened `scripts/smoke_production_acceptance.sh` so production acceptance child-stage logs are captured to same-directory temporary files before publication.
- Added `RELEASE_MANAGED_LOG_TMP_PATH`, `MANAGED_PAYLOADS_LOG_TMP_PATH`, and `WORKFLOW_CLOSURE_LOG_TMP_PATH` cleanup through the existing trap.
- Replaced direct child-stage redirection to final `release-managed.txt`, `managed-payloads.txt`, and `workflow-closure.txt` leaves with redirection to hidden `mktemp` files.
- Rechecked each final stage-log output path before publishing.
- Published each stage log through explicit `mv -f ... || fail "unable to publish ..."` commands, then cleared the corresponding temp path.
- Preserved failed-child diagnostics by publishing the captured child-stage log before reporting the production acceptance failure.

## Safety Boundary

- This milestone only changes local stage-log capture and publication inside the production acceptance harness.
- It does not add evidence signing, append-only storage, payload execution, downloads, package manager calls, provider/worker contact, live target contact, hidden autonomous execution, or a 100% completion claim.
- True completion still depends on real operator-owned evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED static coverage first showed child stages still redirected directly to final `RELEASE_MANAGED_LOG`, `MANAGED_PAYLOADS_LOG`, and `WORKFLOW_CLOSURE_LOG` paths and lacked stage-log `mktemp` plus fail-closed publish commands.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmoke(PublishesStageLogsThroughTempFiles|RejectsSymlinkStageLogOutputPath|PreflightsStageLogOutputPaths|RejectsSymlinkLogDirParent|AllowsDefaultLogDirUnderSymlinkTmpDir|ScriptExercisesOperatorInventoryAndPolicy)' -count=1 -v`, including root package runtime about 60s.
- Broad production/completion/final-evidence regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 118s.
- Focused documentation/static validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(DocumentationBashBlocksPassShellSyntaxCheck|FinalEvidenceRunbookDocumentsStageProvenanceArtifactBindings|ProductionAcceptanceSmoke(PublishesStageLogsThroughTempFiles|RejectsSymlinkStageLogOutputPath|PreflightsStageLogOutputPaths|RejectsSymlinkLogDirParent|AllowsDefaultLogDirUnderSymlinkTmpDir))' -count=1 -v`.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 457s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printing `built bin/ouro`.
- Script syntax passed with `sh -n scripts/audit_completion.sh scripts/smoke_production_acceptance.sh scripts/smoke_managed_payloads.sh scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh`.
- `gofmt -l release_scripts_test.go`, `git diff --check`, and a touched-file trailing-whitespace scan returned clean after final documentation synchronization.

## Review Notes

- A read-only sidecar review was launched for this milestone, but the agent stream disconnected with a transport timeout before returning findings.
- Local RED/GREEN, broad regression, full repository validation, strict readiness, build, and hygiene checks are the authoritative evidence for this slice.
- The implementation reduces direct final-leaf redirection but remains a local file-publication hardening step, not an append-only evidence store or signature system.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
