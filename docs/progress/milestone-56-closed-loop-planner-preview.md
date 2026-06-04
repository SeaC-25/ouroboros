# Milestone 56: Closed-Loop Planner Preview

## Implemented

- Added a bounded `closed-loop-planner-preview` stage to the default `ouro run` path after successful `tool-execution` and before report projection.
- Rebuilds a replay snapshot from already persisted run events and calls `sisyphus.Engine.Step` against the projected Blackboard state.
- Persists metadata-only planner visibility events:
  - `EventAgent` for the closed-loop planner state.
  - `EventAttempt` with `kind=planner_review`, selected task kind, selected intent id when present, reason, `execution_mode=metadata-only`, and `executed_tools=0`.
  - `EventHint` from `closed-loop-planner`.
  - `EventFact(kind=suggestion)` from `closed-loop-planner` so the recommendation appears in replay, the TUI Attack Graph, and Markdown `Next-Run Suggestions`.
- Updated run integration tests to prove planner preview events are durable, replay-projected, report-visible, TUI-visible, and ordered before report projection.

## Boundaries

- This is an operator-review planner preview only.
- It does not compile or execute a second plan.
- It does not dispatch workers or tools.
- It does not contact providers, transports, endpoints, scoped targets, or container runtimes.
- It does not call model generation, route providers, stream tool calls, or perform provider failover.
- It does not mutate worker leases, queue claims, report dispatch artifacts, replay workbench artifacts, or model usage ledgers.

## Validation

- RED app integration failed as expected before implementation with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestRunWorkspaceRecordsDurableEventsReplayAndReport -count=1`, because the Markdown report lacked the closed-loop planner suggestion.
- Focused app integration passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRunWorkspaceRecordsDurableEventsReplayAndReport|TestRunWorkspaceRecordsObservableStageAttemptEvents' -count=1`.
- Full app package passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`.
- Replay/report/TUI/CLI regression passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/eventlog ./internal/report ./internal/tui ./cmd/ouro -count=1`.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`.
- Strict doctor passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`, printing `workspace: ready`.

## Remaining Work

- Closed-loop planner execution remains a follow-up: the preview does not yet feed the selected task back into compiler execution for a second gated tool round.
- Run-integrated model generation/accounting, provider routing/failover, production binary payload bundling, and autonomous distributed worker scheduling beyond bounded/gated audit runs remain outside this milestone.
