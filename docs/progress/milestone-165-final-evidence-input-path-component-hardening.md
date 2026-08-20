# Milestone 165: Final Evidence Input Path-Component Hardening

## Summary

- Hardened `scripts/smoke_production_acceptance.sh` so operator inventory/policy evidence paths and inventory `binary_payload` / `rule_bundle_payload` fields cannot pass through existing symlink path components.
- Hardened `scripts/audit_completion.sh` so top-level production summary and attestation paths, referenced inventory/policy paths, referenced payload fields, and digest/script/stage helper paths reject existing symlink path components.
- Added regression coverage for symlink-parent evidence and payload paths in production acceptance and completion audit.

## Safety Boundary

- This milestone only hardens final evidence input path validation.
- It does not add signing, append-only evidence, payload execution, downloads, package-manager calls, provider/worker/target contact, workspace mutation beyond existing smoke gates, or a 100% completion claim.
- True completion still depends on real operator-owned evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED first showed production acceptance inventory/policy symlink-component paths failed later on schema fields, production acceptance payload symlink-component paths failed later on stage-log tripwires, and completion audit accepted several symlink-component summary/attestation/referenced-evidence/referenced-payload paths when the bytes and digests matched.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(ProductionAcceptanceSmokeRejects(EvidenceInput|Payload)SymlinkPathComponents|CompletionAuditScriptRejects(TopLevelEvidence|ReferencedEvidence|ReferencedPayload)SymlinkPathComponents)' -count=1 -v`.
- Post-review helper coverage passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScriptRejects(StageLog|RecordedScript|StageArtifact)SymlinkPathComponents|Test(ProductionAcceptanceSmokeRejects(EvidenceInput|Payload)SymlinkPathComponents|CompletionAuditScriptRejects(TopLevelEvidence|ReferencedEvidence|ReferencedPayload)SymlinkPathComponents)' -count=1 -v`.
- Adjacent production/completion regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(ProductionAcceptanceSmoke(Rejects(EvidenceInput|Payload|UntrustedWorkspacePath|SymlinkInventoryFile|RelativeEvidencePaths|RelativePayloadInventoryField)|ScriptExercisesOperatorInventoryAndPolicy|AllowsDefaultWorkspaceUnderSymlinkTmpDir)|CompletionAuditScript(Rejects(TopLevelEvidence|ReferencedEvidence|ReferencedPayload|StageWorkspace|SymlinkProductionSummary|RelativeProductionSummaryPath|RelativeReferencedPayloadPath|SymlinkReferencedPayloadPath)SymlinkPathComponents|ValidatesProductionSummaryAndAttestation|RejectsMissingStageWorkspaceArtifacts|RejectsReleaseManagedRuleDirectoryPlaceholder|RejectsManagedPayloadArtifactDigestMismatch|RejectsWorkflowSelectionWithoutCurrentStatus))' -count=1 -v`.
- Final-evidence documentation validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocuments(StageProvenanceArtifactBindings|RequiredFields)|TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Broad production/completion regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 163s.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 363s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printing `built bin/ouro`.
- Script syntax, `gofmt -l release_scripts_test.go`, and `git diff --check` returned clean.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
