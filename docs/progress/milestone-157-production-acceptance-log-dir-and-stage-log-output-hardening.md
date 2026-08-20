# Milestone 157: Production Acceptance Log Dir and Stage Log Output Hardening

## Summary

- Hardened `scripts/smoke_production_acceptance.sh` so an explicitly configured `OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR` cannot pass through existing symlink path components.
- Kept that path-component refusal scoped to explicitly configured production acceptance log directories, preserving the default unset-log-dir path under symlinked `TMPDIR` locations.
- Hardened production acceptance stage log leaf outputs by defining `RELEASE_MANAGED_LOG`, `MANAGED_PAYLOADS_LOG`, and `WORKFLOW_CLOSURE_LOG`.
- Preflighted `release-managed.txt`, `managed-payloads.txt`, and `workflow-closure.txt` with `assert_output_file_path` after directory creation and before any shell redirection.
- Reused the same stage-log variables for child-stage redirects, failure messages, success-sentinel `grep`, SHA-256 calculation, and production summary fields.

## Safety Boundary

- This milestone only changes local production acceptance evidence output preflight.
- It does not add evidence signing, append-only storage, payload execution, downloads, package manager calls, provider/worker contact, live target contact, hidden autonomous execution, or a 100% completion claim.
- True completion still depends on real operator-owned evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED first showed explicit `OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR=<symlink-parent>/logs` reached later missing-inventory handling and wrote `summary.txt` through the symlinked parent.
- Compatibility RED/GREEN preserved the default unset-log-dir path when `TMPDIR` itself is a symlink, because that default log directory is generated rather than operator-specified final evidence.
- Stage-log RED showed a preexisting symlinked `release-managed.txt` leaf could be opened by shell redirection before the child stage failed, and static coverage showed the script lacked stage-log `assert_output_file_path` calls.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmoke(RejectsSymlinkStageLogOutputPath|PreflightsStageLogOutputPaths|RejectsSymlinkLogDirParent|AllowsDefaultLogDirUnderSymlinkTmpDir|RejectsSymlinkLogDir|ScriptExercisesOperatorInventoryAndPolicy)' -count=1 -v`, including root package runtime about 60s.
- Broad production/completion/final-evidence regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 129s.
- Focused documentation/static validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(DocumentationBashBlocksPassShellSyntaxCheck|FinalEvidenceRunbookDocumentsStageProvenanceArtifactBindings|ProductionAcceptanceSmoke(RejectsSymlinkStageLogOutputPath|PreflightsStageLogOutputPaths|RejectsSymlinkLogDirParent|AllowsDefaultLogDirUnderSymlinkTmpDir))' -count=1 -v`.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 404s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printing `built bin/ouro`.
- Script syntax passed with `sh -n scripts/audit_completion.sh scripts/smoke_production_acceptance.sh scripts/smoke_managed_payloads.sh scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh`.
- `gofmt -l release_scripts_test.go`, `git diff --check`, and a touched-file trailing-whitespace scan returned clean after final documentation synchronization.

## Review Notes

- A read-only sidecar review from Banach identified the stage-log leaf preflight gap and a lower-priority configured-log-dir TOCTOU caveat. The leaf preflight gap is now covered by RED/GREEN tests and implementation.
- A new review agent could not be spawned during this milestone because the agent thread limit was reached. Local RED/GREEN, broad regression, and script hygiene remain the authoritative evidence for this slice.
- The residual TOCTOU caveat is documented as relying on operator-owned configured log directories; this milestone does not attempt append-only or file-descriptor based evidence publication.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
