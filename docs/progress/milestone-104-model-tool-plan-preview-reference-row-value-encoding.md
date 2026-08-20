# Milestone 104 - Model Tool Plan Preview Reference Row-Value Encoding

## Summary

Milestone 104 hardens model tool-plan preview Runtime/TUI rows that carry executable preview-acceptance references and acceptance audit references.

The affected rows now carry adjacent RawURL base64 sidecars:

- `model tool plan preview:` keeps readable `preview=` / `path=` values and adds exact `path_b64=` while preserving `preview_b64=`.
- Replay-derived `model tool plan preview:` rows now emit the same `path_b64=` and `preview_b64=` sidecars.
- `model tool plan preview acceptance artifact:` keeps readable `accepted_by=`, `preview=`, `review=`, `selection=`, `path=`, and `reason=` values, and adds `accepted_by_b64=`, `preview_b64=`, `review_b64=`, `selection_b64=`, and `path_b64=` while preserving `reason_b64=`.

Runtime Tabs now decodes those sidecars for RuntimeAction/Inspector fields and parses machine metadata from rows with legacy text fields stripped first.

## Why

Model tool-plan preview rows are selected by operators and valid persisted preview rows can trigger the explicit double-confirm preview acceptance gate. Before this milestone, the preview artifact path and acceptance references were read from whitespace-delimited `key=value` text.

That meant preview paths, acceptance paths, accepted-by text, review paths, selection paths, or preview artifact references containing spaces, tabs, repeated whitespace, or embedded tokens such as `status=failed`, `mode=execute`, `validity=invalid`, `accepted_by=evil`, or `preview=/tmp/evil.json` could truncate paths or retarget RuntimeAction metadata.

## Changes

- Added compact readable `path=` plus raw `path_b64=` to workspace `model tool plan preview:` rows in `internal/app/tui_snapshot.go`.
- Added `accepted_by_b64=`, `preview_b64=`, `review_b64=`, `selection_b64=`, and `path_b64=` to workspace `model tool plan preview acceptance artifact:` rows in `internal/app/tui_snapshot.go`.
- Added `path_b64=` and `preview_b64=` to replay-derived `model tool plan preview:` rows in `internal/tui/replay.go`.
- Updated `runtimeModelToolPlanPreviewActionForRow` to parse control metadata from preview/path-stripped rows and decode the exact artifact path from `path_b64=`.
- Updated `runtimeModelToolPlanPreviewAcceptanceActionForRow` to parse status/mode/action/counters from reason/reference-stripped rows and decode accepted-by, preview, review, selection, path, and reason sidecars for details.
- Extended runtime row-field boundaries for `path_b64`, `review_b64`, `selection_b64`, `accepted_by_b64`, `accepted_by`, and `accepted_at`.
- Kept short control fields and preview text visible before bulky path sidecars so normal TUI rows still show digest and preview in wide-enough panels.

## Validation

Passed before documentation:

- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeModelToolPlanPreview(UsesEncodedPathField|AcceptanceUsesEncodedReferenceFields)|TestModelWithReplaySnapshotShowsToolPlanPreviewRows' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestModelToolPlanPreviewProjectionIncludesEncodedReferenceFields' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'ModelToolPlanPreview|RuntimeModelToolPlanPreview|ReplaySnapshotShowsToolPlanPreview' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'ModelToolPlanPreview|NewTUIModelForWorkspaceShowsModelToolPlanPreview|NewTUIModelForWorkspaceAcceptsModelToolPlanPreview' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go internal/tui/replay.go internal/tui/replay_test.go`

## Review

A read-only subagent was launched for sidecar-scope review but hit a transport timeout and returned no usable findings. Local RED/GREEN tests and full validation carried the quality gate for this milestone.

## Follow-Ups

The next high-value row-value targets are `compile-preview review artifact:` path/reference fields, report projection dispatch/execution rows, worker lifecycle action-plan/policy rows, worker placement rows, worker lease recovery rows, and install/toolchain artifact rows.

Broader display-only message rows should stay lower priority unless a RuntimeAction parser starts consuming them for executable or Inspector metadata.
