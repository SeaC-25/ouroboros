# Milestone 62: Run-Integrated Model Usage Accounting Preview

Date: 2026-06-04

## Summary

`ouro run` now writes a metadata-only model usage accounting preview during the `closed-loop-planner-preview` stage. The replay payload uses `kind=model_usage_preview`, carries provider/model identity from the prepared provider runtime, records zero token/cost counters for the deterministic planner preview, sets `budget_status=not_enforced`, and records `ledger_write=false`.

The replay projection now exposes these preview records through `ReplaySnapshot.ModelUsagePreviews`, while still aggregating their token/cost counters through the existing model usage ledger projection. The TUI Model Runtime tab renders compact `run model usage preview:` rows, and the Markdown report receives a `kind=suggestion` fact so the accounting preview is visible in `Next-Run Suggestions`.

## Boundary

This milestone does not call provider generation, persist prompts or assistant text, write provider response bodies, write `workspace/runtime/model-usage/`, enforce spend budgets, reserve cost, route or fail over providers, execute another tool round, dispatch workers, or contact scoped targets. It is a run-integrated metadata surface only.

Manual provider preview usage records remain separate under `workspace/runtime/model-usage/` and are still written only through explicit `ouro provider generate --record-usage`.

## Tests

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/eventlog -run 'TestBuildReplaySnapshotProjectsModelUsagePreviews' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRunWorkspaceRecordsClosedLoopPlannerModelUsagePreview' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/eventlog ./internal/tui -run 'Replay|ModelRuntime|RuntimeTabs' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'RunWorkspace.*(ClosedLoopPlanner|DurableEventsReplayAndReport|PlannerSelectedCompilePreview|DeferReportProjection)|ClosedLoopPlanner|ReplayWorkspace' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'RunCommand|ReplayCommand|ProviderUsage|RootCommand' -count=1 -v`

Full validation is pending after documentation synchronization.
