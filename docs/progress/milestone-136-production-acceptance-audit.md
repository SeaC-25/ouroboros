# Milestone 136: Production Acceptance Audit

## Implemented
- Added `scripts/smoke_production_acceptance.sh` and `make smoke-production-acceptance`.
- The harness requires `OURO_PRODUCTION_ACCEPTANCE_INVENTORY` and `OURO_PRODUCTION_ACCEPTANCE_POLICY`.
- The inventory and policy are parsed as fixed `key=value` evidence files and are not sourced as shell.
- Required inventory fields cover schema, `inventory_status=operator_owned`, binary name/pack/path/version/source/expected SHA-256, and rule-bundle name/pack/path/version/source/expected SHA-256.
- Required policy fields cover schema, `policy_status=approved`, deployment environment, accepted-by, accepted-at, authorization scope, required managed-payload and workflow-closure checks, and explicit safe booleans for no live targets, no payload execution, no network downloads, no package managers, no provider calls, and no worker contact.
- The harness runs release-managed acceptance, operator managed-payload acceptance, and workflow-closure acceptance in separate fresh sub-workspaces under the acceptance workspace.
- It writes raw stage logs plus `summary.txt` with audit id, timestamps, accepted/failed status, evidence paths, stage result fields, and SHA-256 digests for the stage logs.
- It refuses an existing non-empty supplied acceptance workspace unless `OURO_PRODUCTION_ACCEPTANCE_SMOKE_ALLOW_EXISTING_WORKSPACE=1` is set.

## Not Implemented
- No operator files are executed or sourced.
- No remote payload downloads, package-manager calls, managed payload execution, provider calls, worker dispatch/contact, or live endpoint/target contact are added.
- No new autonomous runner behavior is added. The harness aggregates existing explicit gates and smoke entrypoints.
- This does not claim the bundled seed is a full third-party corpus; it validates an operator-owned local inventory and deployment policy around the existing acceptance paths.

## Validation
- RED first failed because `smoke-production-acceptance` and `scripts/smoke_production_acceptance.sh` did not exist:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmoke(ScriptExercisesOperatorInventoryAndPolicy|RefusesExistingConfiguredWorkspaceWithoutOptIn)|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`
- A second RED after the initial script passed showed the harness was not yet aggregating release-managed acceptance or writing stage log digests.
- GREEN focused validation passed after adding release-managed acceptance and summary log SHA-256 fields:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestProductionAcceptanceSmoke(ScriptExercisesOperatorInventoryAndPolicy|RefusesExistingConfiguredWorkspaceWithoutOptIn)|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`
- Full repository validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- Strict readiness passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
- Build validation passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- Core workflow smoke passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow`
- Workflow closure smoke passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow-closure`
- Release-managed smoke passed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-release-managed`
- Script syntax, diff hygiene, and touched-file gofmt passed:
  - `sh -n scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_workflow_closure.sh scripts/smoke_release_managed.sh scripts/smoke_managed_payloads.sh scripts/smoke_production_acceptance.sh`
  - `git diff --check`
  - `gofmt -l release_scripts_test.go`

## Risks / Decisions
- The harness intentionally fails closed when the operator inventory or policy is absent, incomplete, or inconsistent. `make smoke-production-acceptance` is not expected to pass without operator evidence.
- Stage workspaces are separated so release-managed package acceptance, local operator payload acceptance, and workflow closure do not depend on each other's side effects.
- The audit summary is line-oriented rather than JSON to match the shell-only harness and keep parsing dependencies minimal.
