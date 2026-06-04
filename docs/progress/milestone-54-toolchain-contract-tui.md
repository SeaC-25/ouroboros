# Milestone 54: Toolchain Contract TUI Health Preview

## Implemented

- Surfaced `CheckToolchainContracts` in workspace-bound TUI Toolchain Health as `toolchain contract:` summary rows.
- Added blocked-contract error rows and read-only Node Inspector details for contract summary/error selections.
- Compact large contract error sets in the TUI: the summary keeps the total `errors=N`, the panel shows the first few error rows, and an omitted-count row points operators back to `ouro toolchain contract-check`.
- Kept the TUI projection metadata-only by switching workspace-bound install request/dispatch/orchestration, compile-preview review, and replay-workbench list/read helpers to skip container runtime checks.
- Updated Runtime Tabs rendering so short panels keep the selected row visible; new health rows no longer hide selected double-confirm results.

## Boundaries

- The TUI contract projection reads workspace manifests and static built-in contracts only.
- It does not execute health probes, dependency commands, installers, tools, providers, workers, transports, scoped targets, container runtime probes, or model generation.
- It does not write artifacts or mutate replay events.
- Full blocked-contract error output remains available through `ouro toolchain contract-check <workspace>`.

## Validation

- RED/GREEN: `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceShows(ToolchainContractRowsMetadataOnly|BlockedToolchainContractRows)' -count=1 -v`
- Focused TUI/app contract regression passed: `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -run 'ToolchainContract|NewTUIModelForWorkspaceShowsToolchainContract|DoesNotRequestReadOnlyRuntimeActions' -count=1 -v`
- Workspace TUI projection regression passed: `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'NewTUIModelForWorkspace' -count=1 -v`
- Related app/TUI/CLI regression passed: `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'Toolchain|RuntimeAction|NewTUIModelForWorkspace|ReplayWorkbench|InstallRequest|InstallDispatch|CompilePreview|RootCommandIncludesProductionCommands' -count=1 -v`

## Remaining Work

- Parser registry enforcement remains a follow-up.
- Closed-loop planner integration, run-integrated model generation/accounting, provider routing/failover, production payload bundling, and autonomous distributed worker scheduling beyond bounded/gated audit runs remain outside this milestone.
