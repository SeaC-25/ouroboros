# Milestone 106 - Compile Preview Review Selection/Comparison Row-Value Encoding

## Summary

Milestone 106 hardens the remaining parser-consumed compile-preview review rows:

- `compile-preview review selection:`
- `compile-preview review comparison:`

Selection rows now keep readable companions and add RawURL base64 sidecars for operator identity, timestamps, reason text, and the selected review reference. Comparison rows now keep readable base/candidate companions and add exact reference sidecars.

## Why

The selection and comparison rows feed RuntimeAction/Inspector details. Before this milestone, whitespace-delimited `review=`, `selected_by=`, `cleared_by=`, `base=`, and `candidate=` fields could be truncated by spaces, tabs, or repeated whitespace. Embedded tokens such as `review=`, `selected_at=`, `base=`, `candidate=`, `status=`, or `mode=` inside readable text could also retarget the parsed RuntimeAction metadata.

## Changes

- Added `selected_by_b64=`, `selected_at_b64=`, `cleared_by_b64=`, `cleared_at_b64=`, `reason_b64=`, and `review_b64=` to `compile-preview review selection:` rows.
- Added `base_b64=` and `candidate_b64=` to `compile-preview review comparison:` rows.
- Updated Runtime Tabs selection parsing to strip legacy reason/review/actor/timestamp text before reading machine fields, then decode sidecars for Inspector details.
- Updated Runtime Tabs comparison parsing to strip legacy base/candidate text before reading mode/action/count metadata, then decode exact base/candidate paths.
- Kept legacy no-sidecar fallback behavior for older rows.
- Adjusted normal-width TUI list assertions so they check stable visible prefixes while projection/parser/detail tests continue to verify full exact paths and reason text, including the cleared-selection projection sidecars.

## Validation

Passed before documentation synchronization:

- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestCompilePreviewReview.*SelectionProjectionIncludesEncodedReferenceFields|TestCompilePreviewReviewComparisonProjectionIncludesEncodedReferenceFields|TestNewTUIModelForWorkspaceShowsCompilePreviewReviewSelection|TestNewTUIModelForWorkspaceShowsClearedCompilePreviewReviewSelection|TestNewTUIModelForWorkspaceSelectsCompilePreviewReviewAfterDoubleConfirm' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeCompilePreviewReview(SelectionUsesEncodedReferenceFields|SelectionUsesEncodedClearedFields|ComparisonUsesEncodedReferenceFields)' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'CompilePreviewReview(Selection|Comparison)|CompilePreviewReview|RuntimeTabs|RuntimeAction|RootCommandIncludesProductionCommands' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Review

A read-only subagent was started for a final selection/comparison row audit while local implementation and validation stayed on the critical path. Earlier milestone-105 review had already identified selection review references and comparison base/candidate references as the next narrow targets.

## Follow-Ups

The next row-value targets are report projection dispatch/execution rows, worker lifecycle action-plan/policy rows, worker placement rows, worker lease recovery rows, and install/toolchain artifact rows. Broader display/message rows remain lower priority unless a RuntimeAction parser consumes them for executable or Inspector metadata.

No release smoke was rerun because release packaging and bundled manifests were not touched.
