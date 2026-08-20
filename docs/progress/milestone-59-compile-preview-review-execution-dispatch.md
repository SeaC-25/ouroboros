# Milestone 59: Compile Preview Review Execution Dispatch

## Implemented

- Added dry-run metadata-only compile-preview review execution dispatch artifacts under `workspace/toolchain/compile-preview-review-execution-dispatches/`.
- Added `CreateToolCompilePreviewReviewExecutionDispatch`, `ListToolCompilePreviewReviewExecutionDispatches`, and `ReadToolCompilePreviewReviewExecutionDispatch`.
- Required the dispatch to reference the current selected compile-preview review and snapshot the review action, node count, warning count, review path, and selection path.
- Added `ExecuteToolCompilePreviewReviewExecutionDispatch`, which reads a persisted dry-run dispatch, rejects stale review summaries, and then reuses the existing strict `ExecuteToolCompilePreviewReview` gate.
- Linked execution audit artifacts back to the dispatch path through `dispatch_path`.
- Added CLI commands:
  - `ouro toolchain compile-preview-review-execution-dispatch <workspace> <review-artifact>`
  - `ouro toolchain compile-preview-review-execution-dispatches <workspace> [artifact]`
  - `ouro toolchain execute-compile-preview-review-execution-dispatch <workspace> <dispatch-artifact> --execute`
- Rendered compile-preview review execution dispatch artifacts as read-only Toolchain Health rows with Node Inspector details.

## Boundaries

- Dispatch creation is metadata-only and does not execute tools, write replay events, mutate worker leases, contact providers, contact transports, or contact scoped targets.
- Dispatch execution still requires an explicit `--execute` flag and reuses the existing strict safety authorization gate before replay events, worker leases, runner calls, or execution audit writes.
- Dispatch execution fails closed if the persisted dispatch no longer matches the current review action, node count, or warning count.
- TUI visibility is read-only in this milestone; there is no TUI execution shortcut for compile-preview review execution dispatch artifacts yet.
- Existing direct `execute-compile-preview-review` remains available, but the dispatch path now provides a stronger auditable review boundary for future IDE execution.

## Validation

- App focused validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Test(CreateToolCompilePreviewReviewExecutionDispatchWritesMetadataOnlyCurrentReviewGate|ExecuteToolCompilePreviewReviewExecutionDispatchRunsReviewedDispatchWithExplicitGate)' -count=1 -v`.
- CLI focused validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'TestToolchain(CompilePreviewReviewExecutionDispatchCommandWritesAndListsMetadataOnlyArtifact|ExecuteCompilePreviewReviewExecutionDispatchCommandRunsWithExecuteFlag)' -count=1 -v`.
- TUI focused validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceShowsCompilePreviewReviewExecutionDispatchRowsReadOnly -count=1 -v` and `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run TestModelUpdateDoesNotRequestReadOnlyRuntimeActions -count=1 -v`.

## Remaining Work

- A later milestone can add double-confirm TUI execution for accepted dispatch artifacts by reusing `ExecuteToolCompilePreviewReviewExecutionDispatch`.
- Run-integrated model generation/accounting, provider routing/failover, production binary payload bundling, fuller managed rule provenance, and autonomous distributed worker scheduling remain outside this milestone.
