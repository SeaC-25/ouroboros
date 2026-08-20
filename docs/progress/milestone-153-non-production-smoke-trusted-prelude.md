# Milestone 153: Non-Production Smoke Trusted Prelude

## Summary

- Hardened `scripts/smoke_workflow.sh`, `scripts/smoke_workflow_closure.sh`, `scripts/smoke_release_managed.sh`, and `scripts/smoke_managed_payloads.sh` so direct execution starts through `/bin/sh`.
- Reset `PATH` to a trusted system path before `ROOT` resolution, temporary directory creation, workspace/log preflight, `go run`, release build/install calls, or CLI smoke commands.
- Added direct-exec regression coverage proving caller-controlled `PATH/sh`, prelude `dirname`, and fake `go` cannot steer those smoke entrypoints.
- Updated the workflow-closure review-link regression fixture so it uses real trusted `go run` against a temporary fake `cmd/ouro` module instead of relying on caller `PATH` fake `go`; the regression still proves `workflow-links review=` paths with spaces bind the compile dispatch review artifact.

## Safety Boundary

- This milestone does not add payload execution, downloads, package manager calls, provider/worker contact, live target contact, hidden autonomous execution, or a 100% completion claim.
- The smoke scripts still exercise only their existing explicit local acceptance paths.

## Validation

- RED first showed all four non-production smoke scripts could be intercepted before real-script preflight by caller-controlled `PATH/sh`.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(ManagedPayloadsSmoke|SmokeWorkflowClosure|ReleaseManagedSmoke|SmokeWorkflow)DirectExecUsesTrustedShellAndPathBeforePrelude' -count=1 -v`.
- Workflow-link regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run TestSmokeWorkflowClosureBindsReviewArtifactFromWorkflowStatusLinks -count=1 -v`.
- Adjacent smoke regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(ManagedPayloadsSmoke|SmokeWorkflowClosure|ReleaseManagedSmoke|SmokeWorkflow)(DirectExecUsesTrustedShellAndPathBeforePrelude|ScriptExercises|RefusesExistingConfiguredWorkspaceWithoutOptIn|Rejects(RelativeWorkspacePath|SymlinkWorkspacePath|RelativeLogDir|SymlinkLogDir|RelativePayloadPath|SymlinkPayloadPath)|BindsReviewArtifactFromWorkflowStatusLinks)' -count=1 -v`, including root package runtime about 286s.
- Broad production/completion/smoke regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAudit|TestProductionAcceptanceSmoke|TestManagedPayloadsSmoke|TestSmokeWorkflow|TestReleaseManagedSmoke|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 454s.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 515s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printing `built bin/ouro`.
- Script syntax passed with `sh -n scripts/audit_completion.sh scripts/smoke_production_acceptance.sh scripts/smoke_managed_payloads.sh scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh`.
- `gofmt -l release_scripts_test.go` and `git diff --check` returned clean before documentation synchronization.

## Review Notes

- Two read-only subagent review attempts were launched for shell-boundary and test-quality review, but both disconnected with transport timeouts before returning findings. The local focused, adjacent, broad, full, doctor, and build validation above are the authoritative evidence for this milestone.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest review, and final human/operator evidence review.
- Follow-up candidates remain evidence-bundle signing/append-only review hardening and any newly discovered final-evidence review ergonomics gaps, without weakening the explicit operator-gated completion boundary.
