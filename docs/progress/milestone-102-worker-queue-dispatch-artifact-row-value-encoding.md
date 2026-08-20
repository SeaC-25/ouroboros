# Milestone 102 - Worker Queue Dispatch Artifact Row-Value Encoding

## Summary

Milestone 102 hardens worker queue dispatch execution/result artifact Runtime/TUI rows.

The affected rows now carry adjacent RawURL base64 sidecars:

- `worker queue dispatch execution artifact:` uses `artifact_b64`, `claim_b64`, `worker_b64`, and `execution_b64`.
- `worker queue dispatch result artifact:` uses `artifact_b64`, `dispatch_b64`, `worker_b64`, and `execution_b64`.

Runtime Tabs decodes the leading artifact path sidecar before action metadata is read, strips readable reference text before parsing `status` and `mode`, and decodes reference sidecars for RuntimeAction/Inspector details.

## Why

Queue dispatch execution/result rows are selected into RuntimeAction details. Before this milestone, their parsers split the leading artifact path at the first space and read references with whitespace-delimited `key=value` parsing.

That meant audit artifact paths, claim/dispatch paths, worker ids, or execution ids containing text such as `status=failed`, `mode=execute`, `claim=/tmp/evil.json`, `dispatch=/tmp/evil.json`, or `worker=evil` could truncate paths or retarget Inspector metadata.

## Changes

- Added `artifact_b64`, `claim_b64`, `worker_b64`, and `execution_b64` to `worker queue dispatch execution artifact:` rows in `internal/app/tui_snapshot.go`.
- Added `artifact_b64`, `dispatch_b64`, `worker_b64`, and `execution_b64` to `worker queue dispatch result artifact:` rows in `internal/app/tui_snapshot.go`.
- Kept compact human-readable companions while encoding raw original values in sidecars, preserving values with tabs or repeated spaces.
- Updated `runtimeWorkerQueueDispatchExecutionArtifactActionForRow` and `runtimeWorkerQueueDispatchResultArtifactActionForRow` to decode leading artifact sidecars, parse machine metadata from reference-stripped rows, and decode reference sidecars.
- Added parser and projection regressions for path/reference injection and compact-readable/raw-sidecar compatibility.

## Validation

Passed before documentation:

- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorkerQueueDispatch(Execution|Result)ArtifactUsesEncodedReferenceFields' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRuntimeWorkerQueueDispatch(Execution|Result)ProjectionIncludesEncodedReferenceFields' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorkerQueueDispatch(Execution|Result)Artifact|TestModelUpdate.*WorkerQueue|TestRuntimeWorkerTransportRequestArtifact' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRuntimeWorkerQueueDispatch(Execution|Result)Projection|TestNewTUIModelForWorkspaceShowsRuntimeWorkerQueueDispatch|TestNewTUIModelForWorkspaceShowsRuntimeWorkerTransport|TestRuntimeWorkerTransportRequestProjection' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Review

A read-only subagent independently recommended the same sidecar keys, parser helper reuse, field mappings, compact readable companions, and legacy fallback strategy. Its targeted failure report matched the RED tests and was addressed by the final implementation.

## Follow-Ups

The next high-value row-value targets are remaining worker dispatch retry reference rows. Broader display-only message rows should stay lower priority unless a RuntimeAction parser starts consuming them.
