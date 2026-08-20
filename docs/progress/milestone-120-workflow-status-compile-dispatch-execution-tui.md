# Milestone 120: Workflow Status Compile Dispatch Execution TUI

Date: 2026-06-07

## Summary

Milestone 120 closes the next model-tool-plan workflow-status gate after compile dispatch creation. A workspace-bound TUI can now execute an existing compile-preview review execution dispatch directly from a `workflow status:` row when the workflow summary is ready for `execute-compile-preview-review-execution-dispatch`.

The action uses the same two-step TUI safety pattern as adjacent execution gates. The first `x` only arms `confirm-required`; the second `x` calls the existing app execution gate and writes the compile execution audit produced by that gate.

## Gate

The row is executable only when all of these are true:

- `stage=model-tool-plan`
- `status=ready`
- `next_gate=execute-compile-preview-review-execution-dispatch`
- `dispatch` is present and is not `-` or `none`

Unsupported workflow rows remain non-executable. The app-level executor revalidates the model-tool-plan stage and dispatch path before calling `ExecuteToolCompilePreviewReviewExecutionDispatch`.

## Boundaries

The TUI action calls:

```go
ExecuteToolCompilePreviewReviewExecutionDispatch(
    context.Background(),
    root,
    dispatchPath,
    ToolCompilePreviewReviewExecutionOptions{
        Execute: true,
        Actor: "tui",
        Reason: "executed from TUI workflow status",
    },
)
```

It writes exactly one compile-preview review execution audit for the selected dispatch when the reviewed frozen nodes execute successfully. It does not create extra compile dispatches, create report projection dispatches or executions, write Markdown reports, or create worker queue/transport artifacts outside the existing frozen-node execution path. The workflow-status safety summary still blocks this row before it becomes executable when strict safety authorization is missing.

After the second confirmation, the UI refreshes Toolchain Health and selects the new `compile-preview review execution artifact:` row.

## Implementation

- `internal/tui/runtime_tabs.go`
  - Allows exact ready workflow-status compile execution rows through `runtimeActionIsReadOnly`.
  - Parses the compile execution dispatch path into both `DispatchPath` and `ArtifactPath` for consistent action details.
  - Keeps blocked rows, wrong stages, and missing dispatch values unsupported.

- `internal/app/tui_snapshot.go`
  - Routes the exact workflow-status compile execution gate through `ExecuteToolCompilePreviewReviewExecutionDispatch`.
  - Uses TUI provenance metadata: `actor=tui`, `reason=executed from TUI workflow status`.

- `internal/tui/app.go`
  - Routes workflow-status compile execution results to Toolchain Health rather than Timeline Lanes.
  - Replaces compile execution rows and selects the resulting execution artifact path.

- `internal/app/tui_snapshot_test.go`
  - Adds workspace-bound double-confirm integration coverage proving first `x` writes nothing and second `x` runs the frozen reviewed node, writes exactly one execution audit linked to the dispatch, and avoids extra compile dispatch, report, Markdown, and worker queue/transport artifacts.

- `internal/tui/runtime_tabs_test.go`
  - Adds Runtime Tabs double-confirm coverage and updates unsupported-row coverage for missing dispatch, `none` dispatch, blocked status, and wrong stage.

## Validation

Focused validation passed before this document:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestModelUpdate(ExecutesCompilePreviewReviewExecutionDispatchFromWorkflowStatusOnlyAfterSecondConfirm|DoesNotExecuteUnsupportedCompileWorkflowStatusRows)' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceExecutesCompilePreviewReviewExecutionDispatchFromWorkflowStatusAfterDoubleConfirm' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -run 'WorkflowStatus|CompilePreviewReviewExecutionDispatch|RuntimeAction' -count=1 -v
```

Final validation passed after documentation synchronization:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow
sh -n scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh
git diff --check
gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/app.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go
```

## Follow-Up

`workflow status:` rows still use whitespace-compacted readable path fields. A later row-value encoding slice should add `_b64` sidecars for workflow-status path fields before relying on those rows in workspaces with spaces in paths.
