# Milestone 117: Workflow Status Report Execution TUI Selection

Date: 2026-06-07

## Summary

Milestone 117 closes the final report-projection workflow shortcut in the workspace-bound TUI. A `workflow status:` row can now select an accepted report projection execution as the current report from Model Runtime, but only for the narrow state where:

- `stage=report-projection`
- `next_gate=select-report-projection-execution`
- `report_execution` is present
- workflow status is `ready`

The action uses the existing two-step `x` confirmation. The first `x` arms confirmation and writes nothing. The second `x` calls `SelectReportProjectionDispatchExecution` with `selected_by=tui` and `reason=selected from TUI workflow status`, refreshes Timeline Lanes, and selects the `report projection execution selection:` row.

## Boundaries

This is current-report marker selection for an existing accepted report projection dispatch execution only. It does not create report projection dispatches, execute dispatches, write Markdown reports, append replay events, rewrite report execution annotations, execute compile-preview dispatches, write compile selection markers, or contact providers, targets, endpoints, workers, leases, tool runners, container runtime, or transport outboxes.

The parser keeps `report_execution` separate from the compile-review `execution` field and uses `report_execution` for the selection gate. Non-report-projection workflow rows, non-ready rows, missing `report_execution`, report dispatch creation/execution/review mismatches, preview acceptance, safety authorization, compile execution, and provider-preview setup remain unsupported/read-only through `x`.

The `workflow-status` CLI command remains read-only.

## Implementation

- `internal/tui/runtime_tabs.go`
  - Allows workflow status execution only for exact ready report-projection create, execute, review, or selection gates.
  - Uses `ReportExecutionPath` / `ExecutionPath` for the selection gate rather than compile execution metadata.
  - Requires explicit `report_execution` for review and selection gates, so generic compile `execution` metadata cannot make those workflow rows executable.

- `internal/app/tui_snapshot.go`
  - Extends the workspace-bound workflow-status executor branch.
  - Keeps report dispatch creation, execution, and review on their existing gates.
  - Adds the selection gate through `SelectReportProjectionDispatchExecution` with fixed TUI selection metadata.
  - Requires the workflow action's `ReportExecutionPath` for review and selection; it does not fall back to generic `ExecutionPath`.

- `internal/tui/app.go`
  - Keeps workflow-status results in Timeline Lanes.
  - Selects `report projection execution selection:` for selection-gate results.

- `internal/app/tui_snapshot_test.go`
  - Adds end-to-end TUI coverage proving first `x` writes nothing and second `x` writes only the report selection marker without creating dispatches, execution audits, Markdown reports, replay events, worker queue claims, or transport requests, and without rewriting the existing report execution annotation.

- `internal/tui/runtime_tabs_test.go`
  - Adds parser/action hardening coverage proving workflow review and selection gates remain read-only when only a generic compile execution path is present.

## Validation

Focused validation passed before this document:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceSelectsReportProjectionExecutionFromWorkflowStatusAfterDoubleConfirm -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run TestWorkflowStatusReportExecutionGatesRequireReportExecutionPath -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'WorkflowStatus|ReportProjection|RuntimeAction|RuntimeTabs|DoesNotExecuteWorkflowStatusRows' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'WorkflowStatus|ReportProjectionDispatch|NewTUIModelForWorkspace.*ReportProjection|NewTUIModelForWorkspace.*WorkflowStatus' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'WorkflowStatus|ReportProject|RootCommandIncludesProductionCommands' -count=1 -v
```

Final validation also passed:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
sh -n scripts/release.sh scripts/build.sh scripts/install.sh
git diff --check
gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/app.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go
```

No release smoke was rerun because release packaging and bundled manifests were not changed.
