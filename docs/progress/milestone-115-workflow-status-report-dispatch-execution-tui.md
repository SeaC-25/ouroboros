# Milestone 115: Workflow Status Report Dispatch TUI Execution

Date: 2026-06-07

## Summary

Milestone 115 closes the next report-projection workflow shortcut in the workspace-bound TUI. A `workflow status:` row can now execute an already-created report projection dispatch from Model Runtime, but only for the narrow state where:

- `stage=report-projection`
- `next_gate=execute-report-projection-dispatch`
- `report_dispatch` is present
- workflow status is `ready`

The action uses the existing two-step `x` confirmation. The first `x` arms confirmation and writes nothing. The second `x` calls `ExecuteReportProjectionDispatch` with `Execute:true`, `actor=tui`, and `reason=executed from TUI workflow status`, writes one report projection execution audit, writes the planned Markdown report, appends the normal report projection replay/report-ready events through the existing app gate, switches to Timeline Lanes, and selects the new `report projection dispatch execution artifact:` row.

## Boundaries

This is execution of an existing report projection dispatch only. It does not call `CreateReportProjectionDispatch`, does not create an extra dispatch artifact, does not execute compile-preview dispatches, and does not contact providers, targets, endpoints, workers, leases, tool runners, container runtime, or transport outboxes.

The parser now keeps `report_dispatch` separate from the compile-review `dispatch` field and uses `report_dispatch` for the execute gate. Non-report-projection workflow rows, non-ready rows, missing `report_dispatch`, preview acceptance, safety authorization, compile execution, report review/selection, and provider-preview setup remain unsupported/read-only through `x`.

The `workflow-status` CLI command remains read-only.

## Implementation

- `internal/tui/runtime_tabs.go`
  - Adds `ReportDispatchPath` to `RuntimeAction`.
  - Parses `report_dispatch` independently from compile `dispatch`.
  - Treats workflow status as executable only for the exact ready report-projection create or execute gate.
  - Points execute-gate `ArtifactPath` and `DispatchPath` at the report dispatch path.

- `internal/app/tui_snapshot.go`
  - Extends the workspace-bound workflow-status executor branch.
  - Keeps the existing create gate on `CreateReportProjectionDispatch`.
  - Adds the execute gate through `ExecuteReportProjectionDispatch` with the explicit `Execute:true` option.

- `internal/tui/app.go`
  - Keeps workflow-status execution results in Timeline Lanes.
  - Selects `report projection dispatch artifact:` for create-gate results.
  - Selects `report projection dispatch execution artifact:` for execute-gate results.

- `internal/tui/runtime_tabs_test.go`
  - Adds parser coverage proving execute-gate workflow status uses `report_dispatch` rather than compile `dispatch` or compile `execution`.

- `internal/app/tui_snapshot_test.go`
  - Adds an end-to-end TUI test proving first `x` writes nothing and second `x` executes the existing report dispatch without creating extra dispatches, worker queue claims, or transport requests.

## Validation

Focused validation passed before this document:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run TestWorkflowStatusExecuteReportProjectionDispatchUsesReportDispatch -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceExecutesReportProjectionDispatchFromWorkflowStatusAfterDoubleConfirm -count=1 -v
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
