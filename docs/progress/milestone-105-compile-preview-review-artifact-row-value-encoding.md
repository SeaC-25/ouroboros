# Milestone 105 - Compile Preview Review Artifact Row-Value Encoding

## Summary

Milestone 105 hardens `compile-preview review artifact:` Runtime/TUI rows that carry executable review-selection and dispatch-creation metadata.

The row now keeps readable companions and adds adjacent RawURL base64 sidecars:

- `reviewer_b64=` for operator/reviewer identity text.
- `reviewed_at_b64=` for reviewed-at text.
- `path_b64=` for the exact persisted compile-preview review artifact path.
- Existing `reason_b64=` remains in place.

Runtime Tabs decodes those sidecars for RuntimeAction/Inspector details and parses status, mode, action, node count, warning count, current marker state, and decision from rows with legacy `reason`, `reviewer`, `reviewed_at`, and `path` text stripped first.

## Why

`compile-preview review artifact:` rows can be selected by operators. Non-current rows can trigger the explicit double-confirm current-review selection gate, and current rows can trigger the explicit double-confirm execution-dispatch creation gate.

Before this milestone, the review artifact path was read from whitespace-delimited `path=` text. Paths with spaces, tabs, repeated whitespace, or embedded tokens such as `status=failed`, `mode=execute`, `reviewed_at=...`, or `path=/tmp/evil.json` could truncate the artifact path or pollute Inspector/action metadata. Reviewer text could also contain `reviewed_at=` and retarget the displayed reviewed timestamp.

## Changes

- Added compact readable `reviewer=`, `reviewed_at=`, and `path=` companions plus exact `reviewer_b64=`, `reviewed_at_b64=`, and `path_b64=` sidecars in `internal/app/tui_snapshot.go`.
- Updated `runtimeCompilePreviewReviewArtifactActionForRow` to prefer `path_b64=`, preserve old no-sidecar `path=` fallback, and parse machine fields from a text-stripped row.
- Updated reviewer and reviewed-at parsing so old rows with only `reason_b64=` do not let `reviewed_at=` text inside the reason override the real timestamp.
- Added `reviewer_b64` and `reviewed_at_b64` as runtime field boundaries.
- Added RED/GREEN projection and parser tests for hostile paths, reviewer text, reviewed-at text, and legacy compatibility.

## Validation

Passed before documentation:

- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestCompilePreviewReviewProjectionIncludesEncodedReferenceFields' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeCompilePreviewReviewArtifactUsesEncodedReferenceFields' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'CompilePreviewReview|CompilePreviewReviewExecution|ModelToolPlanPreview|RuntimeModelToolPlanPreview|RuntimeTabs|RuntimeAction|RootCommandIncludesProductionCommands' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Review

Two read-only subagents reviewed the row scope and regression plan. They confirmed `compile-preview review artifact:` `path_b64` as the narrow next target and identified related follow-ups for selection/comparison references. Local RED/GREEN work also covered reviewer/reviewed-at sidecars because a parser regression proved those text fields could pollute reviewed timestamp metadata.

## Follow-Ups

The next row-value targets are `compile-preview review selection:` review references, `compile-preview review comparison:` base/candidate references, report projection dispatch/execution rows, worker lifecycle action-plan/policy rows, worker placement rows, worker lease recovery rows, and install/toolchain artifact rows.

No release smoke was rerun because release packaging and bundled manifests were not touched.
