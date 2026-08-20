# Milestone 121: Workflow Status Path Sidecars

Date: 2026-06-07

## Summary

Milestone 121 closes the workflow-status row-value gap left after the TUI workflow shortcuts. `workflow status:` rows now keep compact readable path fields for normal display and add RawURL base64 sidecars for exact path recovery when workspace or artifact paths contain spaces.

Runtime Tabs now decodes those sidecars before building workflow-status `RuntimeAction` values, while preserving legacy no-sidecar rows that already use single-token paths.

## Scope

The main workflow-status row now emits sidecars for:

- `preview`
- `acceptance`
- `review`
- `selection`
- `dispatch`
- `execution`
- `report_dispatch`
- `report_execution`
- `report_selection`
- `report`

`report_run` remains a compact identifier field and does not get a path sidecar.

## Implementation

- `internal/app/tui_snapshot.go`
  - Adds `workflowStatusPathRowField`.
  - Emits `key=value key_b64=<rawurl-base64>` for non-empty, non-placeholder workflow-status path fields.
  - Keeps placeholder values such as `-` and `none` sidecar-free.

- `internal/tui/runtime_tabs.go`
  - Adds workflow-status-specific path decoding that trusts validated adjacent sidecars first.
  - Falls back to `runtimeSimpleKeyValueFields` for legacy no-sidecar rows instead of using the broader greedy `runtimeRowField` parser for path fields.
  - Keeps executable workflow gates unchanged: only the exact ready compile/report workflow-status gates implemented in earlier milestones remain executable.

- `internal/app/tui_snapshot_test.go`
  - Extends workflow-status compile dispatch creation and execution integration coverage to workspaces whose paths contain spaces.

- `internal/tui/runtime_tabs_test.go`
  - Adds direct parser coverage for encoded workflow-status path fields.
  - Keeps legacy report-projection workflow-status parsing covered.

## Boundaries

This milestone does not add a new workflow action. It only makes existing workflow-status rows preserve exact path values for the existing TUI gates and Inspector/action details.

It does not accept model previews, create new gate types, bypass strict safety authorization, create report projection artifacts outside existing explicit report gates, dispatch workers, call providers, or contact targets.

## Validation

Focused validation passed before this document:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorkflowStatusActionUsesEncodedPathFields' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspace(Creates|Executes)CompilePreviewReviewExecutionDispatchFromWorkflowStatusAfterDoubleConfirm' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -run 'WorkflowStatus|CompilePreviewReviewExecutionDispatch|ReportProjection|RuntimeAction|RuntimeTabs' -count=1 -v
```

Final validation passed after documentation synchronization:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow
sh -n scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh
git diff --check
gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go
```
