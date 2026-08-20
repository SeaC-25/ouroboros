# Milestone 107 - Report Projection Reference Row-Value Encoding

## Summary

Milestone 107 hardens the parser-consumed report projection rows:

- `report projection dispatch artifact:`
- `report projection dispatch execution artifact:`
- `report projection execution review:`
- `report projection execution selection:`

The rows now keep readable companions and add RawURL base64 sidecars for leading artifact paths, report/dispatch/execution references, actor/reviewer identity text, timestamps, and current/cleared selection references.

## Why

Report projection rows feed RuntimeAction and Inspector metadata. Before this milestone, leading artifact paths and reference fields were parsed from whitespace-delimited text. Paths or operator text containing spaces, tabs, repeated whitespace, or embedded tokens such as `status=`, `mode=`, `dispatch=`, `report=`, `execution=`, `selected_by=`, or `reviewed_at=` could truncate values or retarget parsed report projection metadata.

## Changes

- Added `artifact_b64=`, `planned_report_b64=`, `actor_b64=`, and `created_at_b64=` to `report projection dispatch artifact:` rows while preserving `reason_b64=`.
- Added `artifact_b64=`, `dispatch_b64=`, `report_b64=`, `actor_b64=`, `executed_at_b64=`, `reviewer_b64=`, and `reviewed_at_b64=` to `report projection dispatch execution artifact:` rows while preserving `reason_b64=` and `review_reason_b64=`.
- Added `reviewer_b64=`, `reviewed_at_b64=`, `execution_b64=`, `dispatch_b64=`, `report_b64=`, and `review_b64=` to `report projection execution review:` rows while preserving `review_reason_b64=`.
- Added `selected_by_b64=`, `selected_at_b64=`, `cleared_by_b64=`, `cleared_at_b64=`, `execution_b64=`, `selection_b64=`, `dispatch_b64=`, and `report_b64=` to `report projection execution selection:` rows while preserving `reason_b64=`.
- Updated Runtime Tabs report projection parsers to decode leading artifact path sidecars and parse machine metadata from rows with legacy readable text stripped first.
- Kept old no-sidecar row fallback behavior for legacy report projection rows.
- Added short `report projection selection compact:` and `report projection review compact:` rows so normal-width Timeline Lanes views keep selection actor and review decision/reviewer visible after the new sidecars lengthen detailed rows.
- Updated projection/list tests to bind exact sidecar assertions to the specific artifact, review, and selection rows while normal TUI view assertions check stable compact/path rows.

## Validation

Passed before documentation synchronization:

- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestReportProjection(Dispatch|DispatchExecution|Execution(Cleared)?Selection)ProjectionIncludesEncodedReferenceFields' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeReportProjection(DispatchArtifactUsesEncodedReferenceFields|DispatchExecutionArtifactUsesEncodedReferenceFields|ExecutionReviewUsesEncodedReferenceFields|ExecutionSelectionUsesEncoded(Cleared)?ReferenceFields)' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceShowsReportProjection' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'ReportProjection|ReportProject|RuntimeAction|RuntimeTabs|NewTUIModelForWorkspace|ReplayWorkbench|ReplayCommand|RunCommand|RootCommandIncludesProductionCommands' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go internal/tui/app.go`

## Review

A read-only subagent audited the initial report projection row-value scope and identified the same missing path/reference sidecars, parser fallback risks, and stale normal-width display assertions that this milestone addressed. A final read-only audit found no producer/parser or legacy fallback gaps; its low-risk normal-width visibility finding was fixed with the new selection/review compact rows and RED/GREEN coverage.

## Follow-Ups

The next row-value targets are worker lifecycle action-plan/policy rows, worker placement rows, worker lease recovery rows, and install/toolchain artifact rows. Broader display/message rows remain lower priority unless a RuntimeAction parser consumes them for executable or Inspector metadata.

No release smoke was rerun because release packaging and bundled manifests were not touched.
