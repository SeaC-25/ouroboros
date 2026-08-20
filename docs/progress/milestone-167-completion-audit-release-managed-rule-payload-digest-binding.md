# Milestone 167: Completion Audit Release-Managed Rule-Payload Digest Binding

## Summary

- Hardened `scripts/audit_completion.sh` so the release-managed rule payload directory is content-bound, not only link-shape-bound.
- The completion audit now recomputes canonical record v1 `directory_canonical_records` for `release_managed_workspace/toolchain/managed-bundles/<rule_bundle_name>/payload` and requires it to match operator inventory `rule_bundle_expected_sha256`.
- Successful evidence manifests now record `release_managed_rule_target`, `release_managed_rule_payload_sha256`, and `release_managed_rule_payload_digest_scope=directory_canonical_records` for final human/operator review.
- Added regression coverage for tampering the release-managed rule payload after the stage evidence exists while keeping the expected rule symlink intact.

## Safety Boundary

- This milestone only hardens final evidence validation for one release-managed stage artifact.
- It does not add signing, append-only evidence, downloads, package-manager calls, payload execution, provider/worker/target contact, workspace mutation, or a 100% completion claim.
- True completion still depends on real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED first showed completion audit accepted a tampered release-managed rule payload directory while the rule link still pointed at the expected payload path and the audit reached `completion-audit: ok`.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(RejectsReleaseManagedRulePayloadDigestMismatch|ValidatesProductionSummaryAndAttestation)' -count=1 -v`.
- Adjacent completion-audit artifact/workspace regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(RejectsReleaseManagedRule(PayloadDigestMismatch|LinkSymlinkPathComponents|TargetSymlinkPathComponents|DirectoryPlaceholder)|ValidatesProductionSummaryAndAttestation|RejectsMissingStageWorkspaceArtifacts|RejectsStageWorkspaceSymlinkPathComponents|RejectsStageArtifactSymlinkPathComponents|RejectsManagedPayloadArtifactDigestMismatch|RejectsWorkflowSelectionWithoutCurrentStatus)' -count=1 -v`.
- Final-evidence documentation validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocuments(StageProvenanceArtifactBindings|RequiredFields)|TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Broad production/completion regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 165s.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 365s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printing `built bin/ouro`.
- Script syntax, `gofmt -l release_scripts_test.go`, and `git diff --check` returned clean.
- Two read-only sidecar review attempts did not return usable findings because one agent disconnected with a transport timeout and the other was closed after timing out; local RED/GREEN and validation remain authoritative for this milestone.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
