# Milestone 61: Compile Preview Review Execution Dispatch TUI Create

## Implemented

- Added workspace-bound TUI double-confirm dispatch creation for `current=true` `compile-preview review artifact:` rows in Toolchain Health.
- Reused `CreateToolCompilePreviewReviewExecutionDispatch` from the TUI executor with `actor=tui` and `reason=dispatched from TUI`.
- Refreshed Toolchain Health with compile-preview review execution dispatch rows after creation and selected the new `compile-preview review execution dispatch artifact:` row.
- Kept `current=false` review artifact behavior unchanged: double-confirm `x` still selects that review as current.
- Kept current selection row behavior unchanged: double-confirm `x` still clears the current marker.
- Kept dispatch execution separate: creating a dispatch writes only dry-run metadata and does not execute review nodes.

## Boundaries

- The first `x` only arms `execution status: confirm-required`; it does not write dispatch artifacts.
- The second `x` writes only a dry-run metadata-only dispatch artifact through the existing app creation path.
- Dispatch creation does not run review nodes, write compile-preview review execution audits, append replay execution events, dispatch workers, contact providers, contact transports, or contact scoped targets.
- The resulting dispatch artifact still requires the separate milestone 60 double-confirm execution path before any review node can run.
- This milestone closes the TUI operator review-to-dispatch gap; it does not implement autonomous multi-round planning, run-integrated model generation/accounting, provider routing/failover, production payload bundling, or autonomous distributed scheduling.

## Validation

- Focused TUI validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'CompilePreviewReview|RuntimeTabs|DoesNotRequestReadOnlyRuntimeActions|DoesNotRequestCompilePreviewReviewExecutionDispatchArtifact' -count=1 -v`.
- Focused app validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'CompilePreviewReview|NewTUIModelForWorkspace(ShowsCompilePreviewReview|CreatesCompilePreviewReviewExecutionDispatch|ExecutesCompilePreviewReviewExecutionDispatch|SelectsCompilePreviewReview|ClearsCompilePreviewReview|PersistsCompilePreviewReview)' -count=1 -v`.
- Focused CLI/toolchain regression passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'CompilePreviewReview|Toolchain|RootCommandIncludesProductionCommands' -count=1 -v`.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`.
- Strict workspace readiness passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`, which printed `workspace: ready`.
- `git diff --check` and touched-file `gofmt -l` checks returned clean before final no-edit validation.

## Remaining Work

- True autonomous multi-round planning and run-integrated model generation/accounting remain outside this milestone.
- Provider capability routing/failover, production binary payload bundling, fuller managed rule provenance, and autonomous distributed worker scheduling remain open roadmap items.
