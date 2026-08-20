# Milestone 116: Workflow Status Report Execution TUI Review

Date: 2026-06-07

## Summary

Milestone 116 closes the next report-projection workflow shortcut in the workspace-bound TUI. A `workflow status:` row can now review an already-written report projection execution audit from Model Runtime, but only for the narrow state where:

- `stage=report-projection`
- `next_gate=review-report-projection-execution`
- `report_execution` is present
- workflow status is `ready`

The action uses the existing two-step `x` confirmation. The first `x` arms confirmation and writes nothing. The second `x` calls `AnnotateReportProjectionDispatchExecution` with `decision=accepted`, `reviewer=tui`, and `reason=reviewed from TUI workflow status`, refreshes Timeline Lanes, and selects the `report projection execution review:` row.

## Boundaries

This is review annotation of an existing report projection dispatch execution only. It does not create report projection dispatches, execute dispatches, write Markdown reports, append replay events, write report selection markers, execute compile-preview dispatches, or contact providers, targets, endpoints, workers, leases, tool runners, container runtime, or transport outboxes.

The parser keeps `report_execution` separate from the compile-review `execution` field and uses `report_execution` for the review gate. Non-report-projection workflow rows, non-ready rows, missing `report_execution`, report dispatch creation/execution mismatches, report selection, preview acceptance, safety authorization, compile execution, and provider-preview setup remain unsupported/read-only through `x`.

The `workflow-status` CLI command remains read-only.

## Implementation

- `internal/tui/runtime_tabs.go`
  - Adds `ReportExecutionPath` to `RuntimeAction`.
  - Parses `report_execution` independently from compile `execution`.
  - Treats workflow status as executable only for exact ready report-projection create, execute, or review gates.
  - Points review-gate `ArtifactPath` and `ExecutionPath` at the report execution path.

- `internal/app/tui_snapshot.go`
  - Extends the workspace-bound workflow-status executor branch.
  - Keeps report dispatch creation and execution on their existing gates.
  - Adds the review gate through `AnnotateReportProjectionDispatchExecution` with fixed TUI reviewer metadata.

- `internal/tui/app.go`
  - Keeps workflow-status results in Timeline Lanes.
  - Selects `report projection execution review:` for review-gate results.

- `internal/app/tui_snapshot_test.go`
  - Adds end-to-end TUI coverage proving first `x` writes nothing and second `x` annotates the existing report execution without creating dispatches, execution audits, Markdown reports, selection markers, replay events, worker queue claims, or transport requests.

## Validation

Focused validation passed before this document:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceReviewsReportProjectionExecutionFromWorkflowStatusAfterDoubleConfirm -count=1 -v
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

No release smoke is planned because release packaging and bundled manifests were not changed.
