# Milestone 171: Completion Audit Stage Workspace Relationship Binding

## Summary

- Hardened `scripts/audit_completion.sh` so final completion audit no longer accepts otherwise valid stage workspaces outside the production acceptance root.
- The production summary `release_managed_workspace`, `managed_payloads_workspace`, and `workflow_closure_workspace` fields must now resolve to fixed children of `workspace`: `release-managed-workspace`, `managed-payloads-workspace`, and `workflow-closure-workspace`.
- Regression coverage moves each stage workspace to a sibling directory outside the production workspace, and also to an unexpected child name inside the production workspace, while keeping the stage artifacts self-consistent. Both forged summaries are rejected before ready manifest publication.

## Safety Boundary

- This milestone only hardens final evidence validation.
- It does not execute payloads, download bundles, call package managers, contact providers/workers/targets, mutate stage workspaces, add signing, add append-only storage, or claim 100% completion.
- True completion still depends on real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED first showed completion audit accepted release-managed, managed-payloads, and workflow-closure stage workspaces moved outside the production acceptance root when their required artifacts remained self-consistent.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(RejectsStageWorkspaces(OutsideProductionWorkspace|WithUnexpectedProductionChildName)|ValidatesProductionSummaryAndAttestation)' -count=1 -v`.
- Adjacent completion-audit workspace/artifact regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(Rejects(StageWorkspaces(OutsideProductionWorkspace|WithUnexpectedProductionChildName)|StageWorkspaceSymlinkPathComponents|MissingStageWorkspaceArtifacts|StageArtifactSymlinkPathComponents|ReleaseManagedRule(PayloadDigestMismatch|LinkSymlinkPathComponents|TargetSymlinkPathComponents|DirectoryPlaceholder)|ManagedPayloadArtifactDigestMismatch|TruncatedWorkflowSelectionArtifact|WorkflowSelectionProvenanceMismatch)|ValidatesProductionSummaryAndAttestation)' -count=1 -v`.
- Script syntax passed with `sh -n scripts/audit_completion.sh`.
- Final-evidence documentation validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocuments(StageProvenanceArtifactBindings|RequiredFields)|TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Broad production/completion regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 181s.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 370s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`, printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`, printing `built bin/ouro`.
- Full script syntax, `gofmt -l release_scripts_test.go`, `git diff --check`, and touched-file trailing-whitespace scan returned clean.
- A read-only sidecar review found no blocking issue and requested the unexpected-child-name coverage that is now included.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
