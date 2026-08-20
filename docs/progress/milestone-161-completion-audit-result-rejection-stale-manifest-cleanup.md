# Milestone 161: Completion Audit Result-Rejection Stale Manifest Cleanup

## Summary

- Hardened `scripts/audit_completion.sh` failure cleanup for reused completion-audit log directories.
- After the completion audit log directory itself has been verified, failed audits remove stale regular `completion-evidence-manifest.txt` and `completion-evidence-manifest.sha256` even when `completion-audit.txt` cannot be published.
- Added regression coverage for symlinked and directory `completion-audit.txt` result output leaves with stale ready manifest and digest sidecar files.
- Existing result symlink/directory refusal, manifest symlink/directory refusal, and ordinary failed-audit stale cleanup behavior remain covered.

## Safety Boundary

- This milestone only removes stale regular final-review index files from a verified local log directory on failed completion audit paths.
- It does not delete symlink or non-regular manifest outputs, sign evidence, add append-only storage, execute payloads, contact providers/workers/targets, or claim 100%.
- True completion still depends on real operator-owned evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator review.

## Validation

- RED first showed a symlinked `completion-audit.txt` result output was rejected while stale regular `completion-evidence-manifest.txt` and `completion-evidence-manifest.sha256` remained in the same log directory.
- Sidecar review recommended covering directory result output rejection too; `TestCompletionAuditScriptRejectsDirectoryResultOutputPath` now asserts stale manifest and digest sidecar cleanup for that path.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(RemovesStaleManifestWhenResultOutputRejected|RemovesStaleManifestOnFailure|Rejects(Symlink|Directory)(ResultOutputPath|ManifestOutputPath|ManifestDigestOutputPath)|FailsClosedForFixtureAttestation)' -count=1 -v`.
- Final-evidence documentation validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocuments(StageProvenanceArtifactBindings|RequiredFields)|TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Broad production/completion regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 162s.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 360s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printing `built bin/ouro`.
- Script syntax, `gofmt -l release_scripts_test.go`, `git diff --check`, and a touched-file trailing-whitespace scan returned clean.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
