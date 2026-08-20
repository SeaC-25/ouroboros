# Milestone 151: Smoke Entrypoint Path Hardening

## Summary

- Hardened `scripts/smoke_managed_payloads.sh` so `WORKSPACE` must be absolute and must not be a symlink directory before payload checks or any `go run`.
- Hardened `scripts/smoke_managed_payloads.sh` so `OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR` must be an absolute non-symlink directory before smoke log writes.
- Hardened `scripts/smoke_workflow_closure.sh` so `WORKSPACE` must be absolute and must not be a symlink directory before any workflow smoke command.
- Hardened `scripts/smoke_workflow_closure.sh` so `OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR` must be an absolute non-symlink directory before smoke log writes.

## Safety Boundary

- This milestone does not add payload execution, downloads, package manager calls, provider/worker contact, live target contact, replay/report mutation outside the existing smoke gates, or a 100% completion claim.
- Workflow artifact binding remains a separate follow-up; this slice is limited to smoke entrypoint path preflight.

## Validation

- RED first showed managed-payloads smoke accepted relative/symlink workspaces far enough to reach later payload validation and accepted symlinked log dirs without refusal.
- RED first showed workflow-closure smoke accepted relative/symlink workspaces and relative/symlink log dirs far enough to invoke fake `go`.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(ManagedPayloadsSmokeRejects(RelativeWorkspacePath|SymlinkWorkspacePath|SymlinkLogDir)|SmokeWorkflowClosureRejects(RelativeWorkspacePath|SymlinkWorkspacePath|RelativeLogDir|SymlinkLogDir))' -count=1 -v`.
- Managed-payloads/workflow-closure positive and refusal regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(ManagedPayloadsSmoke(ScriptExercisesOperatorSuppliedPayloads|RefusesExistingConfiguredWorkspaceWithoutOptIn|Rejects(RelativeWorkspacePath|SymlinkWorkspacePath|SymlinkLogDir|RelativePayloadPath|SymlinkPayloadPath))|SmokeWorkflowClosure(ScriptExercisesOperatorGateClosure|RefusesExistingConfiguredWorkspaceWithoutOptIn|Rejects(RelativeWorkspacePath|SymlinkWorkspacePath|RelativeLogDir|SymlinkLogDir)))' -count=1 -v`.
- Broad production/completion/smoke regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestSmokeWorkflowClosure|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 486s.
- Strict readiness passed with `make doctor-strict` printing `workspace: ready`.
- Build validation passed with `make build` printing `built bin/ouro`.
- Script syntax passed with `sh -n scripts/smoke_managed_payloads.sh scripts/smoke_workflow_closure.sh scripts/smoke_production_acceptance.sh scripts/audit_completion.sh scripts/smoke_release_managed.sh scripts/smoke_workflow.sh scripts/release.sh scripts/build.sh scripts/install.sh`.
- `gofmt -l release_scripts_test.go` and `git diff --check` returned clean.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest review, and final human/operator evidence review.
- Follow-up candidates remain: direct trusted-shell/PATH prelude hardening for non-production smoke entrypoints and workflow-closure artifact binding to the just-created workflow links rather than broad single-artifact discovery.
