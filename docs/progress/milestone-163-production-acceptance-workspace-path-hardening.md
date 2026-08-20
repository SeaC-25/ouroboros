# Milestone 163: Production Acceptance Workspace Path Hardening

## Summary

- Hardened `scripts/smoke_production_acceptance.sh` so a provided `WORKSPACE` must be absolute, must not be a symlink, and must not pass through existing symlink path components.
- The production acceptance workspace boundary is checked before operator inventory/policy parsing and before any child acceptance stage starts.
- Default temporary acceptance roots are normalized to a physical path before deriving `WORKSPACE`, keeping default runs fresh while avoiding symlinked `TMPDIR` spellings in final stage workspace paths.
- Added regression coverage for relative workspace paths, symlink workspace leaves, symlink parent components, and omitted `WORKSPACE` under a symlinked `TMPDIR`.

## Safety Boundary

- This milestone only hardens the local production acceptance harness input path that is later recorded into final evidence.
- It does not add tool execution, payload execution, downloads, package-manager calls, provider/worker/target contact, signing, append-only evidence, or a 100% completion claim.
- True completion still depends on real operator-owned evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED first showed relative, symlink leaf, and symlink-parent `WORKSPACE` values reached the later missing-inventory check instead of being rejected as workspace path failures.
- A read-only sidecar review found no implementation-order defect and recommended stronger stage-log absence assertions plus explicit default-workspace coverage; those were added before final validation.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmoke(RejectsUntrustedWorkspacePath|AllowsDefaultWorkspaceUnderSymlinkTmpDir|ScriptExercisesOperatorInventoryAndPolicy|AllowsDefaultLogDirUnderSymlinkTmpDir|RefusesExistingConfiguredWorkspaceWithoutOptIn)' -count=1 -v`.
- Final-evidence documentation validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocuments(StageProvenanceArtifactBindings|RequiredFields)|TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Broad production/completion regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 157s.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 342s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printing `built bin/ouro`.
- Script syntax, `gofmt -l release_scripts_test.go`, `git diff --check`, and a touched-file trailing-whitespace scan returned clean.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
