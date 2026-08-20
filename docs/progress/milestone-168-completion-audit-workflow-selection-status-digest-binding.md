# Milestone 168: Completion Audit Workflow Selection Status Digest Binding

## Summary

- Hardened `scripts/audit_completion.sh` so the workflow report-selection artifact is validated as status evidence, not only as a substring marker.
- The completion audit now extracts all JSON `status` values from `workflow_closure_workspace/reports/report-projection-selection.json`, requires at least one status value, and requires every status value to be `current`.
- Successful evidence manifests now record `workflow_report_selection_sha256` for final human/operator review.
- Added regression coverage for a conflicting selection artifact with top-level `status=cleared` and nested `"status":"current"` that previously reached `completion-audit: ok`.

## Safety Boundary

- This milestone only hardens final evidence validation for one workflow-closure stage artifact.
- It does not add signing, append-only evidence, downloads, package-manager calls, payload execution, provider/worker/target contact, workspace mutation, or a 100% completion claim.
- True completion still depends on real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED first showed completion audit accepted a conflicting report-selection artifact because the old marker check only grepped for `"status":"current"` anywhere in the file.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(RejectsWorkflowSelection(WithConflictingStatus|WithoutCurrentStatus)|ValidatesProductionSummaryAndAttestation)' -count=1 -v`.
- Adjacent completion-audit artifact/workspace regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(RejectsWorkflowSelection(WithConflictingStatus|WithoutCurrentStatus)|RejectsReleaseManagedRule(PayloadDigestMismatch|LinkSymlinkPathComponents|TargetSymlinkPathComponents|DirectoryPlaceholder)|ValidatesProductionSummaryAndAttestation|RejectsMissingStageWorkspaceArtifacts|RejectsStageWorkspaceSymlinkPathComponents|RejectsStageArtifactSymlinkPathComponents|RejectsManagedPayloadArtifactDigestMismatch)' -count=1 -v`.
- Final documentation validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocuments(StageProvenanceArtifactBindings|RequiredFields)|TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Broad production/completion regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 365s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`, printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`, printing `built bin/ouro`.
- Full script syntax, `gofmt -l release_scripts_test.go`, `git diff --check`, and touched-file trailing-whitespace scan returned clean before final documentation synchronization.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
