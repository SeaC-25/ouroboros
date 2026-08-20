# Milestone 170: Completion Audit Workflow Selection Provenance Binding

## Summary

- Hardened `scripts/audit_completion.sh` so final completion audit rejects truncated workflow report-selection markers such as `{"status":"current"}`.
- The workflow report-selection artifact must now be a full metadata-only current selection with execution, dispatch, run id, report path, event count, write, selected-by, and selected-at provenance.
- The linked report path must be `reports/<run_id>.md`, and dispatch/execution paths must be direct JSON files under the workflow workspace report artifact directories.
- The linked execution must be completed, execute-mode, written, accepted, and must match the selection dispatch/run/report/event-count fields.
- The linked dispatch must be dry-run, metadata-only, and must match the selection run/report/event-count fields.
- Successful evidence manifests now record `workflow_report_execution`, `workflow_report_dispatch`, `workflow_report_run_id`, `workflow_report_path`, and `workflow_report_event_count` in addition to `workflow_report_selection_sha256`.

## Safety Boundary

- This milestone only hardens final evidence validation and review indexing.
- It does not execute tools, write reports, mutate workflow artifacts, dispatch workers, contact providers/endpoints/targets, add signing, add append-only storage, or claim 100% completion.
- True completion still depends on real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED first showed a truncated workflow report-selection artifact with only `{"status":"current"}` still reached `completion-audit: ok` and ready evidence publication.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(Rejects(TruncatedWorkflowSelectionArtifact|WorkflowSelectionProvenanceMismatch)|ValidatesProductionSummaryAndAttestation)' -count=1 -v`.
- Script syntax passed with `sh -n scripts/audit_completion.sh`.
- Adjacent completion-audit regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(Rejects(TruncatedWorkflowSelectionArtifact|WorkflowSelectionProvenanceMismatch|WorkflowSelection(WithConflictingStatus|WithoutCurrentStatus)|ReleaseManagedRule(PayloadDigestMismatch|LinkSymlinkPathComponents|TargetSymlinkPathComponents|DirectoryPlaceholder)|ManagedPayloadArtifactDigestMismatch|ReferencedOperatorPayloadDigestMismatch|SummaryPayloadFieldsMismatch)|ValidatesProductionSummaryAndAttestation|RejectsMissingStageWorkspaceArtifacts|RejectsStageWorkspaceSymlinkPathComponents|RejectsStageArtifactSymlinkPathComponents)' -count=1 -v`.
- Final-evidence documentation validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocuments(StageProvenanceArtifactBindings|RequiredFields)|TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Broad production/completion regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 206s.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 372s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`, printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`, printing `built bin/ouro`.
- Full script syntax, `gofmt -l release_scripts_test.go`, `git diff --check`, and touched-file trailing-whitespace scan returned clean.
- A read-only documentation sidecar confirmed the stale documentation locations before synchronization. A read-only code sidecar disconnected with a transport timeout before returning findings, so local RED/GREEN and validation remain authoritative.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
- A likely next narrow hardening slice is binding the production summary `workspace` relationship to the three stage workspace paths instead of only validating each path independently.
