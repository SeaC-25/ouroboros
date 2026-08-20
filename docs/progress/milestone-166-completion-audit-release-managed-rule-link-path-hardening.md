# Milestone 166: Completion Audit Release-Managed Rule-Link Path Hardening

## Summary

- Hardened `scripts/audit_completion.sh` so the release-managed rule symlink artifact check distrusts symlink path components inside the stage workspace.
- The completion audit now rejects symlink components in the `toolchain/rules/<rule_bundle_name>` parent path and in the expected `toolchain/managed-bundles/<rule_bundle_name>/payload` target path before resolving the rule link target.
- Added regression coverage for symlinked `rules` and `managed-bundles` components that previously still reached `ready-for-final-review`.

## Safety Boundary

- This milestone only hardens final evidence validation for one release-managed stage artifact.
- It does not add signing, append-only evidence, downloads, package-manager calls, payload execution, provider/worker/target contact, workspace mutation, or a 100% completion claim.
- True completion still depends on real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED first showed completion audit accepted forged release-managed stage workspaces with symlinked `toolchain/rules` or `toolchain/managed-bundles` components when the final rule link still resolved to the expected payload directory.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScriptRejectsReleaseManagedRule(Link|Target)SymlinkPathComponents' -count=1 -v`.
- Adjacent completion-audit artifact/workspace regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(RejectsReleaseManagedRule(Link|Target)SymlinkPathComponents|RejectsReleaseManagedRuleDirectoryPlaceholder|ValidatesProductionSummaryAndAttestation|RejectsMissingStageWorkspaceArtifacts|RejectsStageWorkspaceSymlinkPathComponents|RejectsStageArtifactSymlinkPathComponents|RejectsManagedPayloadArtifactDigestMismatch|RejectsWorkflowSelectionWithoutCurrentStatus)' -count=1 -v`.
- Final-evidence documentation validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocuments(StageProvenanceArtifactBindings|RequiredFields)|TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Broad production/completion regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 164s.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 362s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printing `built bin/ouro`.
- Script syntax, `gofmt -l release_scripts_test.go`, and `git diff --check` returned clean.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
