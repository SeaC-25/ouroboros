# Milestone 152: Workflow Closure Review Link Binding

## Summary

- Hardened `scripts/smoke_workflow_closure.sh` so compile dispatch creation uses the exact `workflow-links review=` artifact from the immediately preceding `workflow-status` log.
- Removed the old singleton directory discovery path for compile-preview review artifacts.
- Added workflow-link parsing that preserves paths containing spaces by splitting on known workflow-link key boundaries instead of simple whitespace fields.
- Rejected missing, symlinked, or non-regular linked review artifacts before dispatch creation.

## Safety Boundary

- This milestone does not add payload execution, downloads, package manager calls, provider/worker contact, live target contact, hidden autonomous execution, or a 100% completion claim.
- The smoke still follows explicit operator gates and still avoids evaluating human-readable `workflow-command` hints.

## Validation

- RED first showed an opted-in existing workspace with one stale review artifact could steer compile dispatch creation away from the `workflow-status` linked review.
- Focused GREEN passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run TestSmokeWorkflowClosureBindsReviewArtifactFromWorkflowStatusLinks -count=1 -v`.
- Workflow-closure positive/refusal regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestSmokeWorkflowClosure(ScriptExercisesOperatorGateClosure|BindsReviewArtifactFromWorkflowStatusLinks|RefusesExistingConfiguredWorkspaceWithoutOptIn|Rejects(RelativeWorkspacePath|SymlinkWorkspacePath|RelativeLogDir|SymlinkLogDir))' -count=1 -v`.
- Script syntax passed with `sh -n scripts/smoke_workflow_closure.sh`.
- `gofmt -l release_scripts_test.go` returned clean.
- `git diff --check` returned clean.
- Broad workflow/final-evidence regression passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestSmokeWorkflowClosure|TestProductionAcceptanceSmoke|TestCompletionAudit|TestFinalEvidenceRunbook|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`, including root package runtime about 223s.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, including root package runtime about 468s.
- Strict readiness passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printing `workspace: ready`.
- Build validation passed with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printing `built bin/ouro`.

## Remaining

- True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest review, and final human/operator evidence review.
- Follow-up candidates remain: direct trusted-shell/PATH prelude hardening for non-production smoke entrypoints and similar workflow-link binding for later closure artifacts if any future smoke step stops using direct command artifact output.
