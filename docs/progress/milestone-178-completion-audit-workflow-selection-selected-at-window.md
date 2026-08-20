# Milestone 178: Completion Audit Workflow Selection Selected-At Window

## Summary

- Hardened `scripts/audit_completion.sh` so workflow report-selection `selected_at` must fall within the production summary `started_at` / `completed_at` window.
- Added regression coverage proving selections before production start or after production completion fail instead of publishing `ready-for-final-review` evidence.
- Added `workflow_report_selected_at` to successful `completion-evidence-manifest.txt` output for final review.

## Safety Boundary

- This milestone only validates final evidence metadata.
- It does not execute payloads, mutate workspaces, contact providers/workers/targets, add signing, add append-only storage, or claim 100% completion.
- True completion still depends on real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED first showed `selected_at=2026-06-07T23:59:59Z` and `selected_at=2999-06-08T00:01:00Z` still reached `completion-audit: ok`.
- Focused GREEN passed with `sh -n scripts/audit_completion.sh && GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(ValidatesProductionSummaryAndAttestation|RejectsWorkflowSelectionSelectedAtOutsideProductionWindow)' -count=1 -v`.
- Documentation RED first showed `TestFinalEvidenceRunbookDocumentsStageProvenanceArtifactBindings` failed because README/development did not document `selected_at must not be before started_at`.
- Final-evidence documentation validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocuments(StageProvenanceArtifactBindings|RequiredFields)|TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Adjacent completion-audit timestamp/evidence regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(Rejects(WorkflowSelectionSelectedAtOutsideProductionWindow|AttestationReviewedAfterAuditStarted|AttestationReviewedBeforeProductionCompleted|ProductionSummaryCompletedBeforeStarted|InvalidProductionSummaryTimestamps|InvalidReferencedPolicyAcceptedAtTimestamp|InvalidReviewedAtTimestamp|ChangedReferencedInventoryDigest|ChangedReferencedPolicyDigest|StageWorkspaces(OutsideProductionWorkspace|WithUnexpectedProductionChildName)|TruncatedWorkflowSelectionArtifact|WorkflowSelectionProvenanceMismatch)|ValidatesProductionSummaryAndAttestation)' -count=1 -v`.
- Broad production/completion regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 178s.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 377s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`, printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`, printing `built bin/ouro`.
- Full script syntax, `gofmt -l release_scripts_test.go`, `git diff --check`, and touched-file trailing-whitespace scan returned clean.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.
