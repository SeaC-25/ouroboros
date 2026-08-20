# Milestone 70: Workflow Status Next Gate

## Summary

Added a read-only operator workflow projection that answers "what is the next explicit gate?" across the current model/tool/report chain.

## Implemented

- Added `SummarizeWorkspaceWorkflow` in `internal/app` and `ouro workflow-status <workspace>`.
- Added workspace-bound Model Runtime `workflow status:` and `workflow status links:` rows.
- Added Runtime Tabs parsing for `workflow-status` rows and classified them as read-only.
- Covered next gates for:
  - `run-provider-tool-plan-preview`
  - `accept-model-tool-plan-preview`
  - `create-compile-preview-review-execution-dispatch`
  - `execute-compile-preview-review-execution-dispatch`
  - `create-report-projection-dispatch`
  - `execute-report-projection-dispatch`
  - report execution review/selection
  - current report projection review
- Included a `<prompt>` placeholder in provider-preview command hints so the command is not presented as directly runnable without required prompt text.
- Chose latest workflow artifacts by timestamp fields instead of relying on filename order.

## Safety Boundary

This milestone is visibility only. It reads existing preview, acceptance, compile review, dispatch, execution, report, selection, and replay metadata. It does not accept previews, select or clear markers, create dispatches, execute compile-review nodes, write Markdown reports, call providers, dispatch workers, contact endpoints or targets, read token environment values, probe container runtime, write usage records, or append replay events.

## Validation

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'WorkflowStatus|SummarizeWorkspaceWorkflow|NewTUIModelForWorkspaceShowsWorkflowStatusNextGate|DoesNotRequestReadOnlyRuntimeActions|RootCommandIncludesProductionCommands' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `git diff --check`
- `gofmt -l cmd/ouro/main.go cmd/ouro/main_test.go internal/eventlog/sqlite.go internal/app/workflow_status.go internal/app/workflow_status_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`
