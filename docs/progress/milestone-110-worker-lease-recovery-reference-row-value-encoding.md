# Milestone 110 - Worker Lease Recovery Reference Row-Value Encoding

## Summary

Milestone 110 hardens parser-consumed worker lease recovery rows:

- `worker lease reclaim artifact:`
- `worker lease reclaim execution artifact:`
- `worker lease reclaim execution result`
- `worker lease result reconciliation artifact:`
- `worker lease result reconciliation execution artifact:`
- `lease result reconciliation execution result`

The rows now keep readable companions and add RawURL base64 sidecars for leading artifact paths, dispatch paths, worker/run/execution identity, lease paths, result paths, and queue-dispatch references consumed by RuntimeAction and Inspector metadata.

## Why

Worker lease recovery rows feed TUI double-confirm lease mutation gates and read-only Inspector details. Before this milestone, leading artifact paths and lease/reconciliation reference fields were still parsed from whitespace-delimited row values. Paths or identities containing spaces, tabs, repeated whitespace, or embedded tokens such as `status=`, `mode=`, `dispatch=`, `lease_path=`, `result_path=`, `worker=`, or `execution=` could truncate values or retarget parsed worker lease metadata.

## Changes

- Added `artifact_b64=` to `worker lease reclaim artifact:` rows.
- Added `artifact_b64=` and `dispatch_b64=` to `worker lease reclaim execution artifact:` rows.
- Added `run_b64=`, `execution_b64=`, `worker_b64=`, and `lease_path_b64=` to `worker lease reclaim execution result` rows.
- Added `artifact_b64=` to `worker lease result reconciliation artifact:` rows.
- Added `artifact_b64=` and `dispatch_b64=` to `worker lease result reconciliation execution artifact:` rows.
- Added `run_b64=`, `execution_b64=`, `worker_b64=`, `lease_path_b64=`, `result_path_b64=`, and `dispatch_b64=` to `lease result reconciliation execution result` rows.
- Updated Runtime Tabs worker lease recovery parsers to decode leading artifact/reference sidecars and parse machine metadata from rows with legacy readable reference fields stripped first.
- Added `lease_path_b64` and `result_path_b64` to the Runtime Tabs row-field boundary list.
- Added a compact display-only `lease result reconciliation execution identity` row so key run/execution/worker identity stays visible at normal TUI widths after long sidecar-bearing rows.
- Kept old no-sidecar fallback behavior for legacy worker lease recovery rows.

## Validation

Passed before documentation synchronization:

- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorkerLease(Reclaim|ResultReconciliation).*EncodedReferenceFields' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRuntimeWorkerLease(Reclaim|ResultReconciliation)ProjectionIncludesEncodedReferenceFields' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'WorkerLease|LeaseReclaim|LeaseResultReconciliation|RuntimeWorkerLease|RuntimeAction|RuntimeTabs|RootCommandIncludesProductionCommands' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh scripts/build.sh scripts/install.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Review

Two read-only subagent audits were used before implementation. The producer-side audit identified sidecar gaps for reclaim artifacts, reclaim execution artifacts/results, reconciliation artifacts, reconciliation execution artifacts, and reconciliation execution results. The parser-side audit independently recommended decoding leading artifact/dispatch/run/execution/worker plus lease/result path sidecars and parsing machine metadata after stripping legacy readable reference fields. Local RED/GREEN parser and projection coverage verified the same row shapes.

## Follow-Ups

The next row-value targets are install/toolchain artifact rows. Broader display/message rows remain lower priority unless a RuntimeAction parser consumes them for executable or Inspector metadata.

No release smoke was rerun before this documentation update because release packaging and bundled manifests were not touched.
