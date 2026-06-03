# Milestone 53: Toolchain Capability Contract Check

## Implemented

- Added `internal/contracts` as a read-only contract validator across compiler semantic actions, domain pack capabilities, workspace toolchain manifests, built-in tool catalog entries, and strategy pattern contract names.
- Exported compiler action/capability introspection helpers so contract checks do not duplicate the semantic action table.
- Fixed stale semantic mappings to use implemented capabilities such as `web.auth_state_tracking`, `infra.identity_graph`, and `ctf.fast_hypothesis`.
- Aligned built-in domain pack strategy pattern references to the underscore IDs used by built-in YAML patterns.
- Fixed compiler candidate matching so same-pack tool-level candidates do not suppress unmatched manifest-level capabilities such as `ctf.result_proof`.
- Enforced pack-local built-in catalog contracts so a catalog tool capability or dependency must be declared by that tool's selected pack manifest.
- Hardened contract validation so nil domain packs and blank catalog fields fail closed as contract errors instead of being skipped or panicking.
- Added `ouro toolchain contract-check <workspace>` as a read-only operator command. It prints `status=ready` or `status=blocked` plus `contract-error:` rows, uses metadata-only workspace readiness checks, and does not execute tools, dependency health probes, or container runtime probes.

## Missing Work

- The contract check is CLI/app/test visible but not yet surfaced as a TUI Toolchain Health row.
- Manifest parser IDs are still validated only as non-empty at manifest load time; deeper parser registry enforcement remains a follow-up.
- This does not implement closed-loop planning, model generation in `ouro run`, provider routing/failover, or production binary payload bundling.

## Validation

- Baseline before edits: `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- RED: `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/contracts -run TestBuiltInCapabilityContractsAreAligned -count=1 -v`
- RED/GREEN: `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/compiler -run TestCompileKeepsManifestLevelCapabilityWhenSamePackHasToolLevelCandidate -count=1 -v`
- RED/GREEN: `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run TestToolchainContractCheckCommandReportsBlockedContract -count=1 -v`
- RED/GREEN: `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestCheckToolchainContractsDoesNotRunContainerRuntimeDoctor -count=1 -v`
- RED/GREEN: `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/contracts -run 'TestValidateRejectsCatalog(Capability|Dependency)OutsideDeclaredPack|TestValidateRejects(NilDomainPackWithoutPanic|BlankCatalogFields)' -count=1 -v`
- Focused regression passed: `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/compiler ./internal/contracts ./internal/domain ./internal/domain/webapi ./internal/domain/infra ./internal/domain/ctf ./internal/app ./cmd/ouro -run 'Contract|Compile|Pack|ToolchainCommandsInspectWorkspaceManifests|ToolchainContractCheckCommandReportsBlockedContract|RootCommand' -count=1`
- Artifact/app regression after preserving `ctf.artifact_analysis` in `fingerprint_surface` passed: `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'Mobsfscan|ArtifactScope|ToolchainContract|ToolchainCommandsInspectWorkspaceManifests|RootCommand' -count=1`
- Compiler/contracts regression passed: `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/compiler ./internal/contracts -count=1`
- Full suite passed: `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- Strict doctor: `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict` printed `workspace: ready`.
- `git diff --check`
- Touched Go files: `gofmt -l cmd/ouro/main.go cmd/ouro/main_test.go internal/app/toolchain.go internal/compiler/compiler.go internal/compiler/compiler_test.go internal/contracts/contracts.go internal/contracts/contracts_test.go internal/domain/ctf/pack.go internal/domain/infra/pack.go internal/domain/sdk_test.go internal/domain/webapi/pack.go`

## Next Step

Surface the contract-check result in workspace-bound TUI Toolchain Health, then continue toward parser registry enforcement or closed-loop planner integration.

## Risks And Decisions

- The validator allows a capability to be reused by more than one semantic action, but every domain-declared primary action must still map to that capability.
- `contract-check` is intentionally read-only and fails closed on blocked contracts; it does not run dependency health commands, container runtime probes, installers, tools, providers, workers, transports, or scoped targets.
- The CLI uses built-in pattern contract names so installed binaries do not depend on the source tree. Source-level `ValidateBuiltIn` continues to load and validate the real YAML files under `configs/patterns/`.
