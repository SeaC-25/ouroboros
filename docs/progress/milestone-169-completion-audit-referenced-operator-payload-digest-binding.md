# Milestone 169: Completion Audit Referenced Operator Payload Digest Binding

## Summary

- Hardened `scripts/audit_completion.sh` so final completion audit binds the original operator payload evidence, not only imported stage workspace artifacts.
- The audit now requires production summary `binary_name`, `binary_expected_sha256`, `rule_bundle_name`, and `rule_bundle_expected_sha256` fields to match the referenced inventory.
- The referenced operator binary payload is rehashed against `binary_expected_sha256`.
- The referenced operator rule-bundle payload directory is rehashed with canonical record v1 `directory_canonical_records` against `rule_bundle_expected_sha256`.
- Successful evidence manifests now record referenced operator payload paths, SHA-256 values, and rule-bundle digest scope for final human/operator review.

## Safety Boundary

- This milestone only hardens final evidence validation.
- It does not execute payloads, download bundles, call package managers, contact providers/workers/targets, mutate stage workspaces, add signing, add append-only storage, or claim 100% completion.
- True completion still depends on real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED first showed completion audit accepted post-acceptance tampering of the original inventory `binary_payload` file and `rule_bundle_payload` directory, publishing `completion-audit: ok` and ready manifest sidecars.
- RED also showed forged production summary payload identity/digest fields could drift from the referenced inventory and still pass after refreshing the attestation summary digest.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(Rejects(ReferencedOperatorPayloadDigestMismatch|SummaryPayloadFieldsMismatch)|ValidatesProductionSummaryAndAttestation)' -count=1 -v`.
- Adjacent completion-audit artifact/workspace regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(Rejects(ReferencedOperatorPayloadDigestMismatch|SummaryPayloadFieldsMismatch|WorkflowSelection(WithConflictingStatus|WithoutCurrentStatus)|ReleaseManagedRule(PayloadDigestMismatch|LinkSymlinkPathComponents|TargetSymlinkPathComponents|DirectoryPlaceholder)|ManagedPayloadArtifactDigestMismatch|ChangedReferencedInventoryDigest|ChangedReferencedPolicyDigest|ReferencedPayloadSymlinkPathComponents|RelativeReferencedPayloadPath)|ValidatesProductionSummaryAndAttestation|RejectsMissingStageWorkspaceArtifacts|RejectsStageWorkspaceSymlinkPathComponents|RejectsStageArtifactSymlinkPathComponents)' -count=1 -v`.
- Final documentation validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocuments(StageProvenanceArtifactBindings|RequiredFields)|TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Broad production/completion regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 171s.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 368s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`, printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`, printing `built bin/ouro`.
- Full script syntax, `gofmt -l release_scripts_test.go`, `git diff --check`, and touched-file trailing-whitespace scan returned clean.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
- Sidecar reviews identified workflow report-selection schema/provenance binding as a likely next narrow final-evidence hardening slice.
