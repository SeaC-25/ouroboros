# Milestone 58: Compile Preview Review Execution Gate

## Implemented

- Added `ExecuteToolCompilePreviewReview` to execute the frozen `review.Preview.Nodes` from a persisted compile-preview review.
- Required an explicit execution option and a current matching `workspace/toolchain/compile-preview-review-selection.json` marker before execution.
- Re-ran strict current safety authorization before replay events, worker leases, runner calls, or execution audit writes.
- Persisted execution audits under `workspace/toolchain/compile-preview-review-executions/`.
- Added `ListToolCompilePreviewReviewExecutions` and `ReadToolCompilePreviewReviewExecution` as metadata-only read/list paths.
- Added CLI commands:
  - `ouro toolchain execute-compile-preview-review <workspace> <artifact> --execute`
  - `ouro toolchain compile-preview-review-executions <workspace> [artifact]`
- Rendered compile-preview review execution artifacts as read-only Toolchain Health rows with Node Inspector details.
- Kept TUI execution triggering out of this milestone; execution remains a CLI/app explicit gate.

## Boundaries

- Selection is required plan identity, not safety authorization.
- Execution still fails closed when disabled target policy or disabled isolation lacks current matching `authorization.yaml` metadata.
- The execution path does not recompile the review; it uses the persisted frozen nodes.
- Metadata-only review, annotation, comparison, selection, clear, list, and read paths still do not execute tools.
- TUI rows for execution artifacts are read-only and do not expose `x`/`r` execution shortcuts.
- Execution artifact read/list paths reject symlinked execution files, and execution rejects symlinked execution directories before runner or replay side effects.
- This milestone does not add a dispatch artifact, browser execution route, provider/model generation integration, provider failover, autonomous scheduling, or live target contact beyond the explicitly executed tool plan.

## Validation

- Focused app/TUI validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -run 'Test(ExecuteToolCompilePreviewReview|ReadAndListToolCompilePreviewReviewExecutions|NewTUIModelForWorkspaceShowsCompilePreviewReviewExecutionRowsReadOnly|ModelUpdateShowsSelectedCompilePreviewReviewExecutionDetails|ModelUpdateDoesNotRequestReadOnlyRuntimeActions)' -count=1 -v`.
- CLI focused validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'CompilePreviewReview|Toolchain' -count=1 -v`.
- Touched-package validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1`.

## Remaining Work

- A separate dispatch/gate artifact can make TUI execution safer and more auditable in a later milestone.
- Run-integrated model generation/accounting, provider routing/failover, production binary payload bundling, fuller managed rule provenance, and autonomous distributed worker scheduling remain outside this milestone.
