# Milestone 134: Managed Payload Acceptance Smoke

## Implemented
- Added `scripts/smoke_managed_payloads.sh` and `make smoke-managed-payloads`.
- The smoke accepts operator-supplied local binary and rule-bundle payloads through environment variables:
  - `OURO_MANAGED_BINARY_PAYLOAD`
  - `OURO_MANAGED_BINARY_EXPECTED_SHA256`
  - `OURO_MANAGED_RULE_BUNDLE_PAYLOAD`
  - `OURO_MANAGED_RULE_BUNDLE_EXPECTED_SHA256`
- Optional environment variables override names, packs, versions, and source labels; defaults target `httpx` and `nuclei-templates` in `web-api-pack`.
- The script creates a fresh workspace by default and refuses an existing non-empty supplied workspace unless `OURO_MANAGED_PAYLOADS_SMOKE_ALLOW_EXISTING_WORKSPACE=1` is set.
- The smoke runs strict doctor, imports both payloads with expected digest pins, validates metadata-only install plans, verifies no-`--execute` installs fail without links, executes only the workspace-local link gate, checks final symlink targets point inside `workspace/toolchain/managed-bundles/`, and reads both bundles back.
- Release script regression tests now cover the Makefile entrypoint, a full paths-with-spaces operator-payload smoke, log artifacts, final workspace payload/link state, and refusal before invoking `go` for existing non-empty workspaces.

## Not Implemented
- No managed payload is downloaded, generated, unpacked from remote archives, installed through package managers, or executed.
- No `ouro run`, replay, report projection, provider call, worker dispatch, endpoint contact, target contact, update download execution, update apply execution, or health probe is part of this smoke.
- The script does not claim the checked payloads are a broad third-party payload corpus; it is an acceptance harness for operator-supplied local artifacts.

## Validation
- RED first failed because `scripts/smoke_managed_payloads.sh` and `smoke-managed-payloads` did not exist:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestManagedPayloadsSmoke(ScriptExercisesOperatorSuppliedPayloads|RefusesExistingConfiguredWorkspaceWithoutOptIn)|TestReleaseMakefileAndCIEntrypoints' -count=1 -v`
- GREEN focused validation passed after adding the script, Make target, link-target assertions, and regression coverage with the same command.
- Root package validation passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -count=1`.
- Full repository validation passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`.
- Strict readiness passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printed `workspace: ready`.
- Build validation passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printed `built bin/ouro`.
- Core workflow smoke passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow` ended with `workflow-smoke: ok`.
- Release-managed acceptance smoke passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-release-managed` ended with `release-managed-smoke: ok`.
- Script syntax, diff hygiene, and touched-file gofmt passed with `sh -n scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_release_managed.sh scripts/smoke_managed_payloads.sh`, `git diff --check`, and `gofmt -l release_scripts_test.go`.

## Risks / Decisions
- `run --tui` is intentionally excluded from this acceptance smoke because it enters normal run/replay/report paths and may consume installed managed binaries in configured workspaces. Existing `make smoke-workflow` remains the core TUI/run smoke.
- The managed payload smoke requires reviewed local payload paths and expected digests. It does not synthesize trust in third-party binaries or rules.
