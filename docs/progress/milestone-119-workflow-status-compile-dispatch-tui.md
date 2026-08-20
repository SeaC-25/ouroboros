# Milestone 119: Workflow Status Compile Dispatch TUI

Date: 2026-06-07

## Summary

Milestone 119 closes the model-tool-plan side of the workflow-status operator loop. A workspace-bound TUI can now create a compile-preview review execution dispatch directly from a `workflow status:` row when the next gate is ready for `create-compile-preview-review-execution-dispatch`.

The action keeps the same two-step TUI safety pattern used by adjacent workflow gates: the first `x` only arms `confirm-required`, and the second `x` writes one dry-run metadata-only compile-preview review execution dispatch artifact through the existing app gate.

## Gate

The row is executable only when all of these are true:

- `stage=model-tool-plan`
- `status=ready`
- `next_gate=create-compile-preview-review-execution-dispatch`
- `review` is present and is not `-` or `none`

Unsupported workflow rows remain non-executable. The app-level executor revalidates the same review boundary before calling `CreateToolCompilePreviewReviewExecutionDispatch`.

## Boundaries

The TUI action writes only a dispatch artifact under `workspace/toolchain/compile-preview-review-execution-dispatches/` with:

- `actor=tui`
- `reason=created from TUI workflow status`
- `status=dry-run`
- `mode=metadata-only`

It does not execute compile review nodes, write compile execution audits, create report projection dispatches or executions, write Markdown reports, append replay events, create worker queue or transport artifacts, call providers, or contact targets.

After the second confirmation, the UI refreshes Toolchain Health and selects the new `compile-preview review execution dispatch artifact:` row.

## Implementation

- `internal/tui/runtime_tabs.go`
  - Allows only exact ready `workflow-status` rows for the compile dispatch creation gate.
  - Keeps other model-tool-plan workflow rows read-only/unsupported.

- `internal/app/tui_snapshot.go`
  - Routes the exact workflow-status compile dispatch gate through `CreateToolCompilePreviewReviewExecutionDispatch`.
  - Revalidates missing, `-`, and `none` review values before reaching the app gate.

- `internal/tui/app.go`
  - Routes workflow-status compile dispatch results to Toolchain Health rather than Timeline Lanes.
  - Replaces dispatch rows and selects the new compile dispatch artifact.

- `internal/app/tui_snapshot_test.go`
  - Adds workspace-bound double-confirm integration coverage proving first `x` writes nothing and second `x` creates exactly one metadata-only dispatch without compile execution, report, worker, provider, or target side effects.
  - Adds direct executor coverage that `review=none` fails before writing dispatch artifacts.

- `internal/tui/runtime_tabs_test.go`
  - Adds Runtime Tabs double-confirm coverage and unsupported-row coverage for missing review, blocked status, and compile execution next gates.

## Validation

Focused validation passed before this document:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestTUIRuntimeActionExecutorRejectsWorkflowStatusCompileDispatchWithoutReview|TestNewTUIModelForWorkspaceCreatesCompilePreviewReviewExecutionDispatchFromWorkflowStatusAfterDoubleConfirm' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestModelUpdateCreatesCompilePreviewReviewExecutionDispatchFromWorkflowStatusOnlyAfterSecondConfirm|TestModelUpdateDoesNotExecuteUnsupportedCompileWorkflowStatusRows|TestModelUpdateDoesNotExecuteWorkflowStatusRows' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -run 'WorkflowStatus|CompilePreviewReviewExecutionDispatch|ReportProjection|ModelToolPlanPreview|RuntimeAction|RuntimeTabs' -count=1 -v
```

Final validation passed after documentation synchronization:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow
```
