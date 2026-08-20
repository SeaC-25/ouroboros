# Milestone 196 - Production Acceptance IDE Smoke

## Summary

Added the local IDE usability smoke to the production acceptance and final completion evidence chain. The final production acceptance path now proves the built IDE path and local fake-provider AI path in addition to release-managed, operator managed-payload, and workflow-closure stages.

## Behavior

- `scripts/smoke_production_acceptance.sh` builds a workspace-confined `ouro` binary for the IDE stage.
- The harness runs `scripts/smoke_ide.sh` under `<workspace>/ide-smoke-workspace`.
- The IDE stage log is published as canonical `ide-smoke.txt` through the same temporary-file and fail-closed move pattern used by other stage logs.
- `summary.txt` now records `ide_smoke_workspace`, `ide_smoke_script`, `ide_smoke_script_sha256`, `ide_smoke_acceptance`, `ide_smoke_log`, and `ide_smoke_log_sha256`.
- Failed production acceptance runs remove stale unpublished `ide-smoke.txt` logs from reused log directories.
- `scripts/audit_completion.sh` requires `ide_smoke_acceptance=passed`, the expected `scripts/smoke_ide.sh` digest, canonical `ide-smoke.txt`, the `ide-smoke: ok` marker, and the fixed `<workspace>/ide-smoke-workspace` child before publishing ready final evidence.
- Successful completion audit result/manifest evidence exposes IDE smoke log and workspace bindings for final review.

## Safety Boundary

The IDE stage uses `smoke_ide.sh`, which runs direct-use and AI-provider smoke paths. The AI-provider smoke uses a local fake OpenAI-compatible server, so this milestone proves provider wiring without allowing real provider calls. Production acceptance still requires `provider_calls_allowed=false`, `no_live_targets=true`, and no payload execution, package manager use, worker contact, endpoint contact, or hidden autonomous execution.

## Validation

RED first proved production acceptance and completion audit could pass without IDE smoke evidence. GREEN validation then passed:

```bash
sh -n scripts/smoke_production_acceptance.sh scripts/audit_completion.sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmoke(PreflightsStageLogOutputPaths|PublishesStageLogsThroughTempFiles|RemovesStale)|TestCompletionAuditScript(ValidatesProductionSummaryAndAttestation|RejectsStageLogWithoutOKSentinel|RequiresCanonicalStageLogPaths|RejectsNonCanonicalStageLogFileNames|RejectsStageWorkspaces(OutsideProductionWorkspace|WithUnexpectedProductionChildName))' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmokeScriptExercisesOperatorInventoryAndPolicy' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocuments(StageProvenanceArtifactBindings|RequiredFields)|TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmoke|TestCompletionAuditScript|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
sh -n scripts/smoke_production_acceptance.sh scripts/audit_completion.sh scripts/smoke_ide.sh scripts/smoke_direct_use.sh scripts/smoke_ai_provider.sh scripts/smoke_release_managed.sh scripts/smoke_managed_payloads.sh scripts/smoke_workflow_closure.sh
gofmt -l release_scripts_test.go
git diff --check
```

The full repository validation passed with root package runtime about 415s. `make build` printed `built bin/ouro`; `make doctor-strict` printed `workspace: ready`.

## Remaining

This closes the final-evidence gap where production acceptance could omit proof of local IDE and AI dialog/provider plumbing. It still does not claim 100% completion: true completion requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
