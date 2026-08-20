# Milestone 60: Compile Preview Review Execution Dispatch TUI

## Implemented

- Added workspace-bound TUI double-confirm execution for dry-run metadata-only `compile-preview review execution dispatch artifact:` rows in Toolchain Health.
- Added `compile-preview-review-execution-dispatch-artifact` to the executable runtime action set while keeping non-dry-run dispatch rows read-only.
- Reused `ExecuteToolCompilePreviewReviewExecutionDispatch` from the TUI executor with `actor=tui` and `reason=executed from TUI`.
- Refreshed Toolchain Health with compile-preview review execution audit rows after execution and selected the new `compile-preview review execution artifact:` row.
- Kept `r` request handling unsupported for compile-preview review execution dispatch rows, so dispatch execution remains behind the two-step `x` confirmation path.
- Updated Inspector details for dry-run metadata-only dispatch rows to show `mode: metadata-only` instead of read-only.

## Boundaries

- The first `x` only arms `execution status: confirm-required`; it does not execute review nodes.
- The second `x` delegates to the existing app execution gate, so stale-dispatch checks, current-review validation, strict safety authorization, frozen-node execution, and audit writes remain centralized.
- TUI dispatch execution does not add a new provider, worker, transport, target-contact, request, or arbitrary-command input path.
- Non-dry-run dispatch artifacts and compile-preview review execution audit artifacts stay read-only in the TUI.
- This milestone closes the operator TUI dispatch execution shortcut; it does not implement autonomous multi-round planning, run-integrated model generation/accounting, provider routing/failover, production payload bundling, or autonomous distributed scheduling.

## Validation

- Focused TUI validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'CompilePreviewReviewExecutionDispatch|RuntimeTabs|DoesNotRequestReadOnlyRuntimeActions|DoesNotRequestCompilePreviewReviewExecutionDispatchArtifact' -count=1 -v`.
- Focused app validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'CompilePreviewReviewExecutionDispatch|NewTUIModelForWorkspaceExecutesCompilePreviewReviewExecutionDispatch|NewTUIModelForWorkspaceShowsCompilePreviewReviewExecutionDispatch' -count=1 -v`.
- Focused CLI/toolchain regression passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'CompilePreviewReview|Toolchain|RootCommandIncludesProductionCommands' -count=1 -v`.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`.
- Strict workspace readiness passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`, which printed `workspace: ready`.
- `git diff --check` and touched-file `gofmt -l` checks returned clean before final no-edit validation.

## Remaining Work

- True autonomous multi-round planning and run-integrated model generation/accounting remain outside this milestone.
- Provider capability routing/failover, production binary payload bundling, fuller managed rule provenance, and autonomous distributed worker scheduling remain open roadmap items.
