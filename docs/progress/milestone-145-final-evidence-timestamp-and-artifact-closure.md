# Milestone 145: Final Evidence Timestamp And Artifact Closure

## Implemented
- Added RFC3339-shaped timestamp validation for production policy `accepted_at` and completion attestation `reviewed_at`.
- Anchored completion-audit stage script provenance to the expected repository smoke scripts instead of accepting arbitrary digest-matched script paths.
- Aligned release-managed stage artifact validation with the real workspace contract by requiring `toolchain/rules/<rule_bundle_name>` to be a symlink to the workspace managed rule payload directory.
- Bound the managed-payload stage artifact to the operator inventory by rehashing `toolchain/managed-bundles/<binary_name>/payload/<binary_name>` against `binary_expected_sha256`.
- Required workflow closure evidence to include a current report-selection marker with `status=current`.
- Strengthened final evidence runbook tests so runbook bash blocks pass `sh -n` and docs list the concrete stage provenance keys and artifact paths.

## Not Implemented
- This still does not cryptographically sign evidence bundles or move evidence into append-only storage.
- This still does not replace final human/operator evidence review.
- This still does not claim 100% without real operator-owned production evidence and approved attestation.

## Validation
- RED tests first showed invalid timestamps, fake digest-matched stage scripts, directory rule placeholders, tampered managed binary artifacts, and cleared workflow selections could reach or obscure final review boundaries.
- Focused RFC3339 validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(ProductionAcceptanceSmokeRejectsInvalidAcceptedAtTimestamp|CompletionAuditScriptRejectsInvalid(ReferencedPolicyAcceptedAt|ReviewedAt)Timestamp)' -count=1 -v`
- Focused stage artifact and script-path validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(ValidatesProductionSummaryAndAttestation|RejectsReleaseManagedRuleDirectoryPlaceholder|RejectsManagedPayloadArtifactDigestMismatch|RejectsWorkflowSelectionWithoutCurrentStatus|RejectsWrongStageScriptPathWithMatchingDigest)' -count=1 -v`
- Final evidence runbook validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbook(BashBlocksPassShellSyntaxCheck|DocumentsStageProvenanceArtifactBindings|DocumentsRequiredFields|BashBlocksAreCopyPasteSafe)' -count=1 -v`
- Broad production/completion regression passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestFinalEvidenceRunbook|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`
- Full repository validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- Strict readiness and build validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- Hygiene checks returned clean:
  - `sh -n scripts/audit_completion.sh scripts/smoke_production_acceptance.sh scripts/smoke_managed_payloads.sh scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh`
  - `gofmt -l release_scripts_test.go`
  - `git diff --check`

## Next Step
- Keep the full usable IDE target just short of 100% until real operator-owned evidence and approved attestation pass final audit plus manual review.
