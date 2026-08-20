# Milestone 142: Production Evidence Content Binding

## Implemented
- Hardened `scripts/smoke_production_acceptance.sh` so accepted production summaries include `inventory_sha256` and `policy_sha256` for the exact operator inventory and deployment-policy bytes.
- Hardened `scripts/audit_completion.sh` so completion audit requires those summary digest fields, rehashes the referenced inventory/policy files, rejects post-summary evidence drift, and emits both digests in `completion-audit.txt`.
- Updated the final evidence runbooks to tell operators not to edit or replace referenced inventory/policy files after the production summary is written.

## Not Implemented
- This does not make current payload files the immutable proof of tool execution; stage logs and their SHA-256s remain the execution proof.
- This does not create real operator-owned production evidence or an approved attestation.
- This does not make the final completion gate claim 100%; it remains `ready-for-final-review` until real evidence is reviewed.

## Validation
- RED tests first showed production summaries lacked `inventory_sha256` / `policy_sha256`, completion audit results omitted those digests, and post-summary inventory/policy content drift still reached `completion-audit: ok`.
- Focused GREEN validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmokeScriptExercisesOperatorInventoryAndPolicy|TestCompletionAuditScript(ValidatesProductionSummaryAndAttestation|RejectsChangedReferencedInventoryDigest|RejectsChangedReferencedPolicyDigest)' -count=1 -v`
- Runbook digest documentation validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocumentsRequiredFields|TestProductionAcceptanceSmokeScriptExercisesOperatorInventoryAndPolicy|TestCompletionAuditScript(ValidatesProductionSummaryAndAttestation|RejectsChangedReferencedInventoryDigest|RejectsChangedReferencedPolicyDigest)' -count=1 -v`
- Broad production/completion regression passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbookDocumentsRequiredFields|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`
- Hygiene checks returned clean:
  - `sh -n scripts/audit_completion.sh scripts/smoke_production_acceptance.sh scripts/smoke_managed_payloads.sh scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh`
  - `gofmt -l release_scripts_test.go`
  - `git diff --check`

## Next Step
- Keep the full usable IDE target just short of 100% until real operator-owned evidence and approved attestation pass the final audit and manual review.
