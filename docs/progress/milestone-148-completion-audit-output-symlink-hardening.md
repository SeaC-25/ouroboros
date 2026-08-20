# Milestone 148: Completion Audit Output Symlink Hardening

## Implemented
- Rejected symlinked `OURO_COMPLETION_AUDIT_LOG_DIR` before completion audit writes.
- Rejected preexisting symlinked or non-regular `completion-audit.txt` result output paths before writing failed or successful results.
- Rejected preexisting symlinked or non-regular `completion-evidence-manifest.txt` output paths before writing the manifest.
- Published successful result and manifest bytes through same-directory temporary files before renaming to the final output paths.
- Preserved failed `completion-audit.txt` output for safe result paths, including the symlink-manifest refusal case.
- Updated README and development runbooks to state that the completion audit log directory must not be a symlink and result/manifest output files must be regular files or absent.

## Not Implemented
- This does not cryptographically sign evidence bundles.
- This does not move evidence into append-only storage.
- This does not try to solve hostile same-directory race conditions beyond refusing preexisting symlink handles and avoiding direct result/manifest redirection to final paths.
- This does not replace real operator-owned production evidence, approved attestation, `make audit-completion`, manifest review, or final human/operator evidence review.
- This does not claim 100% completion.

## Validation
- RED first showed that symlinked completion-audit log directory, symlinked result output, symlinked manifest output, directory result output, and directory manifest output scenarios still reached `completion-audit: ok` or an uncontrolled hash error.
- Focused symlink-output validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScriptRejects(Symlink|Directory)(LogDir|ResultOutputPath|ManifestOutputPath)' -count=1 -v`
- Focused success/failure completion-audit validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(ValidatesProductionSummaryAndAttestation|FailsClosedForFixtureAttestation|RejectsSymlink(LogDir|ResultOutputPath|ManifestOutputPath))' -count=1 -v`
- Focused runbook documentation validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run TestFinalEvidenceRunbookDocumentsStageProvenanceArtifactBindings -count=1 -v`
- Broad production/completion regression passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`
- Full repository validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- Strict readiness and build validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- Script syntax, Go formatting, and diff hygiene checks passed:
  - `sh -n scripts/audit_completion.sh scripts/smoke_production_acceptance.sh scripts/smoke_managed_payloads.sh scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh`
  - `gofmt -l release_scripts_test.go`
  - `git diff --check`

## Next Step
- Keep the full usable IDE target at about 99.99% until real operator-owned evidence and approved attestation pass final audit plus manifest/manual review.
