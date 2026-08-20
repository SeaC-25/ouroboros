# Milestone 109 - Worker Placement Reference Row-Value Encoding

## Summary

Milestone 109 hardens parser-consumed worker placement and placement-orchestration rows:

- `worker placement artifact:`
- `worker placement dispatch artifact:`
- `worker orchestration artifact:`
- `worker orchestration supervisor artifact:`

The rows now keep readable companions and add RawURL base64 sidecars for leading artifact paths plus placement/orchestration reference paths consumed by RuntimeAction and Inspector metadata.

## Why

Worker placement rows feed TUI double-confirm actions and read-only Inspector details. Before this milestone, leading artifact paths and orchestration reference fields were still parsed from whitespace-delimited row values. Paths containing spaces, tabs, repeated whitespace, or embedded tokens such as `status=`, `mode=`, `transport=`, `placement_plan=`, or `orchestration_path=` could truncate values or retarget parsed worker placement metadata.

## Changes

- Added leading `artifact_b64=` to `worker placement artifact:` rows.
- Added leading `artifact_b64=` to executable `worker placement dispatch artifact:` rows.
- Added `artifact_b64=`, `placement_plan_b64=`, `placement_dispatch_b64=`, `placement_batch_b64=`, `lifecycle_maintenance_b64=`, and `lifecycle_audit_b64=` to `worker orchestration artifact:` rows.
- Added `artifact_b64=` and `orchestration_path_b64=` to `worker orchestration supervisor artifact:` rows.
- Updated Runtime Tabs placement/orchestration parsers to decode leading artifact sidecars and parse machine metadata from rows with legacy readable reference fields stripped first.
- Added `placement_plan_b64`, `placement_dispatch_b64`, `placement_batch_b64`, `lifecycle_maintenance_b64`, `lifecycle_audit_b64`, and `orchestration_path_b64` to the Runtime Tabs row-field boundary list.
- Kept old no-sidecar fallback behavior for legacy worker placement and orchestration rows.

## Validation

Passed before documentation synchronization:

- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorker(Placement|Orchestration).*Encoded.*ReferenceFields' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRuntimeWorker(Placement|Orchestration).*EncodedReferenceFields' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'WorkerPlacement|PlacementDispatch|WorkerOrchestration|RuntimeWorkerPlacement|RuntimeWorkerOrchestration|RuntimeTabs|RuntimeAction|RootCommandIncludesProductionCommands' -count=1 -v`

## Review

A read-only subagent audited the producer side and identified the same placement artifact, placement dispatch artifact, worker orchestration artifact, and worker orchestration supervisor artifact sidecar gaps addressed here. A second parser-side audit agent timed out before completion, so local RED/GREEN parser coverage and related RuntimeAction regression carried the final parser check.

## Follow-Ups

The next row-value targets are worker lease recovery rows, then install/toolchain artifact rows. Broader display/message rows remain lower priority unless a RuntimeAction parser consumes them for executable or Inspector metadata.

No release smoke was rerun before this documentation update because release packaging and bundled manifests were not touched.
