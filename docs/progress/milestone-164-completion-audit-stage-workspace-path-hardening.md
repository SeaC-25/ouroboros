# Milestone 164: Completion Audit Stage Workspace Path Hardening

## Summary

- Hardened `scripts/audit_completion.sh` so stage workspace paths recorded in production acceptance summaries cannot pass through existing symlink path components.
- The completion audit now applies this boundary to `workspace`, `release_managed_workspace`, `managed_payloads_workspace`, and `workflow_closure_workspace` before checking key stage artifacts.
- Added regression coverage that rewrites otherwise valid production summaries to point at the same artifacts through a symlinked parent path.

## Safety Boundary

- This milestone only hardens final evidence validation for workspace paths consumed from `summary.txt`.
- It does not add tool execution, payload execution, downloads, package-manager calls, provider/worker/target contact, signing, append-only evidence, workspace mutation, or a 100% completion claim.
- True completion still depends on real operator-owned evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED first showed `workspace`, `managed_payloads_workspace`, and `workflow_closure_workspace` symlink-component paths could still reach `completion-audit: ok`, while `release_managed_workspace` failed later on symlink target comparison instead of the intended workspace boundary.
- A read-only sidecar review found the core implementation order correct and recommended asserting failed symlink-component audits do not leave ready manifests; that assertion was added before final validation.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(RejectsStageWorkspaceSymlinkPathComponents|ValidatesProductionSummaryAndAttestation|RejectsMissingStageWorkspaceArtifacts|RejectsReleaseManagedRuleDirectoryPlaceholder|RejectsManagedPayloadArtifactDigestMismatch|RejectsWorkflowSelectionWithoutCurrentStatus)' -count=1 -v`.
- Final-evidence documentation validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocuments(StageProvenanceArtifactBindings|RequiredFields)|TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Broad production/completion regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 163s.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 360s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printing `built bin/ouro`.
- Script syntax, `gofmt -l release_scripts_test.go`, `git diff --check`, and a touched-file trailing-whitespace scan returned clean.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
