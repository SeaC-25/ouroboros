# Milestone 150: Final Evidence Stale Output Hardening

## Summary

- Hardened `scripts/smoke_production_acceptance.sh` so the production acceptance log directory must not be a symlink.
- Hardened production acceptance `summary.txt` output so preexisting symlink or non-regular paths are rejected before accepted or failed summaries are written.
- Published accepted and failed production summaries through same-directory `mktemp` files before rename.
- Hardened `scripts/audit_completion.sh` so a failed audit removes a stale regular `completion-evidence-manifest.txt` from a reused log directory after writing the failed result.

## Safety Boundary

- This milestone does not execute managed payloads, add downloads, call package managers, contact providers/workers/targets, mutate replay/report state, sign evidence, or claim 100%.
- The completion state remains `ready-for-final-review`; true 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest review, and final human/operator evidence review.

## Validation

- RED first showed symlinked production acceptance log directories and symlinked `summary.txt` outputs were not refused before the script reported missing inventory.
- RED also showed a failed completion audit could leave a stale regular `completion-evidence-manifest.txt` in a reused log directory.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmokeRejectsSymlink(LogDir|SummaryOutputPath)' -count=1 -v`.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run TestCompletionAuditScriptRemovesStaleManifestOnFailure -count=1 -v`.
- Combined final-evidence output regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(ProductionAcceptanceSmokeRejectsSymlink(LogDir|SummaryOutputPath)|CompletionAuditScript(RemovesStaleManifestOnFailure|FailsClosedForFixtureAttestation|ValidatesProductionSummaryAndAttestation|Rejects(Symlink|Directory)(LogDir|ResultOutputPath|ManifestOutputPath)))' -count=1 -v`.
- Broad production/completion regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 494s.
- Strict readiness passed with `make doctor-strict` printing `workspace: ready`.
- Build validation passed with `make build` printing `built bin/ouro`.
- Script syntax passed with `sh -n scripts/smoke_production_acceptance.sh scripts/audit_completion.sh scripts/smoke_managed_payloads.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh scripts/release.sh scripts/build.sh scripts/install.sh`.
- `gofmt -l release_scripts_test.go` and `git diff --check` returned clean.

## Remaining

- Real operator-owned production evidence, approved attestation, `make audit-completion`, manifest review, and final human/operator evidence review are still required before claiming the IDE target is 100%.
- Sidecar audit found additional follow-up hardening candidates in `scripts/smoke_managed_payloads.sh` and `scripts/smoke_workflow_closure.sh`: stricter `WORKSPACE`/log-directory path handling and workflow artifact binding.
