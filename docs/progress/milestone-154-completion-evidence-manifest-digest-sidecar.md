# Milestone 154: Completion Evidence Manifest Digest Sidecar

## Summary

- Extended successful `scripts/audit_completion.sh` runs to publish `completion-evidence-manifest.sha256` beside `completion-audit.txt` and `completion-evidence-manifest.txt`.
- Added `evidence_manifest_digest=<path>` to `completion-audit.txt` so final reviewers have a direct pointer to the manifest digest sidecar.
- The sidecar records `result=<path>`, `result_sha256=<sha256>`, `evidence_manifest=<path>`, and `evidence_manifest_sha256=<sha256>` after the result and manifest are already published, avoiding circular digest binding.
- Hardened digest sidecar output handling so symlinked and non-regular `completion-evidence-manifest.sha256` paths fail closed before success, successful publication uses same-directory `mktemp`, and failed audits remove stale regular manifest and digest sidecar files in reused log directories.

## Safety Boundary

- The sidecar is a final-review digest index only.
- This milestone does not add evidence signing, append-only storage, payload execution, downloads, package manager calls, provider/worker contact, live target contact, hidden autonomous execution, or a 100% completion claim.
- True completion still depends on real operator-owned evidence, approved attestation, `make audit-completion`, manifest/sidecar review, and final human/operator review.

## Validation

- RED first showed successful completion audit output lacked `completion-audit: evidence-manifest-digest`, `completion-audit.txt` lacked `evidence_manifest_digest=<path>`, symlinked and directory digest sidecar output paths were accepted through `completion-audit: ok`, and failed reruns left a stale regular digest sidecar in a reused log directory.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(ValidatesProductionSummaryAndAttestation|Rejects(Symlink|Directory)ManifestDigestOutputPath|FailsClosedForFixtureAttestation|RemovesStaleManifestOnFailure)' -count=1 -v`.
- Broad production/completion/final-evidence regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 162s.
- Focused documentation/static validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(CompletionAuditScriptUsesUnpredictableOutputTempFiles|FinalEvidenceRunbookDocumentsStageProvenanceArtifactBindings|DocumentationBashBlocksPassShellSyntaxCheck|CompletionAuditScript(ValidatesProductionSummaryAndAttestation|Rejects(Symlink|Directory)ManifestDigestOutputPath|FailsClosedForFixtureAttestation|RemovesStaleManifestOnFailure))' -count=1 -v`.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 463s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printing `built bin/ouro`.
- Script syntax passed with `sh -n scripts/audit_completion.sh scripts/smoke_production_acceptance.sh scripts/smoke_managed_payloads.sh scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh`.
- `gofmt -l release_scripts_test.go` and `git diff --check` returned clean before final documentation synchronization.

## Review Notes

- A read-only subagent review identified the same digest sidecar output-path and stale-output coverage gaps, plus recommended static temp-name and runbook-documentation checks. The implementation adds those checks locally; final verification remains authoritative.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest and digest sidecar review, and final human/operator evidence review.
- Follow-up candidates remain evidence-bundle signing or append-only review hardening only if they are explicitly scoped; the current sidecar intentionally does not provide those guarantees.
