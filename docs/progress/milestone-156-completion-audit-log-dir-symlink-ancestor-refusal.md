# Milestone 156: Completion Audit Log Dir Symlink Ancestor Refusal

## Summary

- Hardened `scripts/audit_completion.sh` so an explicitly configured `OURO_COMPLETION_AUDIT_LOG_DIR` cannot pass through existing symlink path components.
- Added a path-component walk before and after `mkdir -p` for explicitly configured completion audit log directories.
- Preserved the existing direct symlink log-directory refusal while adding coverage for paths such as `/tmp/link/logs`, where `link` points outside the reviewed tree and `logs` itself is not a symlink.
- Kept the default unset-log-dir path compatible with symlinked `TMPDIR` locations, since those are generated temporary paths rather than operator-specified final evidence directories.

## Safety Boundary

- This milestone only changes local completion audit output path preflight.
- It does not add evidence signing, append-only storage, payload execution, downloads, package manager calls, provider/worker contact, live target contact, hidden autonomous execution, or a 100% completion claim.
- True completion still depends on real operator-owned evidence, approved attestation, `make audit-completion`, manifest/sidecar review, and final human/operator review.

## Validation

- RED first showed explicit `OURO_COMPLETION_AUDIT_LOG_DIR=<symlink-parent>/logs` reached `completion-audit: ok` and wrote result, manifest, and digest sidecar evidence through the symlinked parent.
- A compatibility RED then showed the initial implementation rejected an unset log dir created under a symlinked `TMPDIR`.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(RejectsSymlinkLogDirParent|AllowsDefaultLogDirUnderSymlinkTmpDir|RejectsSymlinkLogDir|ValidatesProductionSummaryAndAttestation)' -count=1 -v`.
- Broad production/completion/final-evidence regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 126s.
- Focused documentation/static validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(DocumentationBashBlocksPassShellSyntaxCheck|FinalEvidenceRunbookDocumentsStageProvenanceArtifactBindings|CompletionAuditScript(RejectsSymlinkLogDirParent|AllowsDefaultLogDirUnderSymlinkTmpDir|RejectsSymlinkLogDir|ValidatesProductionSummaryAndAttestation))' -count=1 -v`.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 395s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printing `built bin/ouro`.
- Script syntax passed with `sh -n scripts/audit_completion.sh scripts/smoke_production_acceptance.sh scripts/smoke_managed_payloads.sh scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh`.
- `gofmt -l release_scripts_test.go` and `git diff --check` returned clean before final documentation synchronization.

## Review Notes

- A read-only subagent review identified the symlinked-`TMPDIR` compatibility risk. Follow-up test coverage and implementation now scope symlink-component refusal to explicitly configured log dirs while keeping the default temporary log-dir path compatible.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
