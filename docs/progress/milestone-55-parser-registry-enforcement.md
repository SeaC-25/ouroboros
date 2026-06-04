# Milestone 55: Parser Registry Contract Enforcement

## Implemented

- Extended `internal/contracts.Validate` to use the platform parser registry as part of the read-only contract boundary.
- Workspace manifest `parser` values now fail contract validation when the parser id is not registered.
- Built-in executable catalog parser ids now fail contract validation when the parser id is not registered.
- Built-in executable catalog parser ids now fail contract validation when the declared adapter operation does not support that parser id.
- `CheckToolchainContracts` reports parser drift as `status=blocked`, so CLI `ouro toolchain contract-check` and workspace-bound TUI Toolchain Health share the same signal.

## Not Implemented

- This does not add runtime parser promotion for every registered parser.
- This does not change `ouro run` parser dispatch behavior; unsupported parser ids still remain non-fatal skips unless a specific parser family has an implemented dispatcher.
- This does not execute parser code, tool health checks, tools, installers, providers, workers, transports, container runtime probes, or scoped targets.
- This does not package production tool binaries or rule corpora.

## Validation

- Baseline before edits passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`.
- RED tests failed as expected for unknown workspace manifest parser, unknown catalog parser, and registered parser unsupported by the declared catalog adapter operation.
- Focused contract validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/contracts -run 'Parser|Catalog|Manifest|BuiltInCapabilityContracts' -count=1 -v`.
- App contract validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'CheckToolchainContracts' -count=1 -v`.
- Related app/CLI regression passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/contracts ./internal/app ./cmd/ouro -run 'Parser|Contract|ToolchainContract|CheckToolchainContracts|RootCommandIncludesProductionCommands' -count=1`.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`.
- Strict doctor passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`, printing `workspace: ready`.
- `git diff --check` and touched-file `gofmt -l` returned no output.

## Next Step

Continue toward closed-loop planner integration, run-integrated model generation/accounting, provider routing/failover, production binary payload bundling, or autonomous distributed worker scheduling beyond bounded/gated audit runs.

## Risks and Decisions

- The parser registry is enforced at the contract-check boundary rather than inside `toolchain.LoadManifest`, so metadata inspection remains schema-oriented while operator health can fail closed on parser drift.
- Registered parsers and runtime parser promotion are intentionally separate. Some registered parser ids are catalog contracts for future tool families and should not make default runs fail before their safe evidence-promotion boundary exists.
