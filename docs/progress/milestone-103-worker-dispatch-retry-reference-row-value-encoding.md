# Milestone 103 - Worker Dispatch Retry Reference Row-Value Encoding

## Summary

Milestone 103 hardens worker dispatch retry execution Runtime/TUI rows that expose retry execution artifacts and retry handoff references.

The affected rows now carry adjacent RawURL base64 sidecars:

- `worker dispatch retry execution artifact:` uses `artifact_b64` and `retry_dispatch_b64`.
- `worker dispatch retry execution result:` uses `source_result_b64`, `source_dispatch_b64`, `source_claim_b64`, `retry_claim_b64`, `queue_dispatch_b64`, `request_path_b64`, `worker_b64`, `run_b64`, and `execution_b64`, while preserving the existing `reason_b64`.

Runtime Tabs decodes the leading artifact sidecar before action metadata is read, strips readable reference text before parsing `status`, `mode`, and `transport`, and decodes reference sidecars for RuntimeAction/Inspector details.

## Why

Worker dispatch retry execution rows are selected into RuntimeAction details. Before this milestone, the retry execution artifact parser split the leading artifact path at the first space, and the execution result parser read path and identity references with whitespace-delimited `key=value` parsing.

That meant retry execution artifact paths, retry-dispatch paths, source result/dispatch/claim paths, retry claim paths, queue-dispatch paths, request paths, worker ids, run ids, or execution ids containing text such as `status=failed`, `mode=metadata-only`, `transport=http-json`, `worker=evil`, or `execution=evil` could truncate paths or retarget Inspector metadata.

## Changes

- Added `artifact_b64` and `retry_dispatch_b64` to `worker dispatch retry execution artifact:` rows in `internal/app/tui_snapshot.go`.
- Added `source_result_b64`, `source_dispatch_b64`, `source_claim_b64`, `retry_claim_b64`, `queue_dispatch_b64`, `request_path_b64`, `worker_b64`, `run_b64`, and `execution_b64` to `worker dispatch retry execution result:` rows in `internal/app/tui_snapshot.go`.
- Kept compact human-readable companions while encoding raw original values in sidecars, preserving values with tabs or repeated spaces.
- Updated `runtimeWorkerDispatchRetryDispatchExecutionArtifactActionForRow` and `runtimeWorkerDispatchRetryDispatchExecutionResultActionForRow` to parse machine metadata from reference-stripped rows and decode sidecars for details.
- Extended runtime field boundaries for the new sidecar keys so legacy field extraction stops before encoded sidecars.
- Added parser and projection regressions for path/reference injection and compact-readable/raw-sidecar compatibility.

## Validation

Passed before documentation:

- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorkerDispatchRetryExecution(Artifact|Result)UsesEncodedReferenceFields' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestWorkerDispatchRetryExecutionProjectionIncludesEncodedReferenceFields' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorkerDispatchRetry|TestRuntimeWorkerQueueDispatch|TestRuntimeWorkerTransportRequest' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestWorkerDispatchRetry|TestNewTUIModelForWorkspaceShowsWorkerDispatchRetry|TestRuntimeWorkerQueueDispatch|TestRuntimeWorkerTransportRequest' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Review

A read-only subagent scan confirmed this milestone excluded already-completed worker transport request, worker queue dispatch, and worker dispatch retry rows, then identified the next highest-risk RuntimeAction rows that still use whitespace-delimited path/reference fields.

## Follow-Ups

The next high-value row-value target is the `model tool plan preview:` / acceptance family, where executable preview acceptance still relies on readable `path`, `review`, and `selection` fields without exact sidecar recovery.

After that, continue through `compile-preview review artifact:`, report projection dispatch/execution rows, worker lifecycle action-plan/policy rows, worker placement rows, worker lease recovery rows, and install/toolchain artifact rows. Broader display-only message rows should stay lower priority unless a RuntimeAction parser starts consuming them.
