# Milestone 51: Provider Usage Summary TUI Preview

## Summary

Milestone 51 surfaces the metadata-only manual provider-preview usage summary in the workspace-bound TUI.

The Model Runtime tab now shows `provider usage summary:` rows built from `workspace/runtime/model-usage/` through the existing `SummarizeProviderUsageRecords` path. Selecting the row and pressing `d` opens read-only Node Inspector details for record counts, provider/model counts, token totals, available cost totals, and the usage ledger source path.

## What Changed

- Added workspace-bound Model Runtime rows for provider usage summary aggregation.
- Reused `SummarizeProviderUsageRecords`, so list/read path-safety checks stay centralized.
- Added `provider usage summary:` runtime-row parsing and read-only RuntimeAction details.
- Added TUI tests for summary visibility, metadata-only content, no replay-event mutation, no usage-record creation, and request rejection for read-only provider summary rows.
- Encoded the usage ledger source path and carried provider/model counts as stable fields so Inspector details remain correct when workspace paths or provider/model names contain spaces or `|`.

## Safety and Scope Boundaries

- TUI summary rows are read-only and do not write workspace state or usage records.
- TUI summary rows do not mutate replay events or mix manual-preview ledger data into replay `model_usage`.
- TUI summary rows do not persist prompt text, assistant text, bearer tokens, provider response bodies, or generated text.
- TUI summary rows do not contact providers, scoped targets, workers, transports, or tools.
- Manual-preview usage records are still not treated as `ouro run` model consumption; this remains an operator visibility bridge before automatic run/replay accounting.

## Validation

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceShowsProviderUsageSummaryReadOnly|TestNewTUIModelForWorkspaceProviderUsageSummaryDoesNotMutateReplayEvents|TestNewTUIModelForWorkspaceProviderUsageSummaryDetailsHandleSpacePathsAndNames' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestModelUpdateDoesNotRequestReadOnlyRuntimeActions' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'ProviderUsage|NewTUIModelForWorkspaceShowsProviderUsage|RuntimeTabs|ModelUpdateDoesNotRequestReadOnlyRuntimeActions|Provider|RootCommand' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict` printed `workspace: ready`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Remaining Gaps

The IDE is still not complete. Current effective gaps include binary production payload bundling, fuller managed rule corpus packaging/provenance, replay/run-integrated model accounting and budget enforcement, streaming/tool-call/provider-failover model runtime integration, model capability routing, generation integration into actual run/agent planning, and autonomous distributed worker scheduling beyond bounded/gated audit runs.
