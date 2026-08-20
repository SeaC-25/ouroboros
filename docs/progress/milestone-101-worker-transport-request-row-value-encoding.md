# Milestone 101 - Worker Transport Request Row-Value Encoding

## Summary

Milestone 101 hardens executable `worker transport request:` Runtime/TUI rows.

The row now carries adjacent RawURL base64 sidecars:

- `request_b64`
- `dispatch_b64`
- `claim_b64`
- `worker_b64`
- `execution_b64`

Runtime Tabs decodes the leading request path sidecar before action metadata is read, strips readable dispatch/claim/worker/execution text before parsing request `status` and `transport`, and decodes reference sidecars for RuntimeAction/Inspector details.

## Why

`worker transport request:` rows are executable from the TUI. Pending rows can be acknowledged with double-confirm `x`, and acknowledged rows can be executed through the same workspace-bound run-once gate. The executor passes the parsed request path and worker id into `ReadRuntimeWorkerTransportDispatchRequest`, `AcknowledgeRuntimeWorkerTransportDispatchRequest`, and `ExecuteRuntimeWorkerTransportDispatchRequest`.

Before this milestone, the parser split the leading request path at the first space and read reference fields with whitespace-delimited `key=value` parsing. A request path, dispatch path, claim path, worker id, or execution id containing text such as `status=failed`, `transport=http-json`, `worker=evil`, or `claim=/tmp/evil.json` could truncate the request path or retarget action metadata.

## Changes

- Added `request_b64`, `dispatch_b64`, `claim_b64`, `worker_b64`, and `execution_b64` to `worker transport request:` rows in `internal/app/tui_snapshot.go`.
- Kept human-readable companions, but compacted their whitespace while encoding the raw original values in sidecars so tabs or repeated spaces still decode safely.
- Updated `runtimeWorkerTransportRequestArtifactActionForRow` to decode the leading request path sidecar, parse status/transport from reference-stripped rows, and decode dispatch/claim/worker/execution sidecars.
- Added `request_b64`, `dispatch_b64`, and `claim_b64` to the generic Runtime Tabs field boundary list.
- Added parser and projection regressions for request/reference injection, compact-readable/raw-sidecar compatibility, and simple no-sidecar legacy fallback.

## Validation

Passed before documentation:

- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorkerTransportRequestArtifactUsesEncodedReferenceFields' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRuntimeWorkerTransportRequestProjectionIncludesEncodedReferenceFields' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorkerTransportRequestArtifact(UsesEncodedReferenceFields|KeepsLegacyReferenceFields)' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

The full test run uses `GOTMPDIR=/home/zss/t` because `/tmp` has limited free space in this environment and longer temporary paths can trigger existing TUI width assumptions unrelated to this milestone.

## Review

A read-only spec review found no issues. A code-quality review found one medium issue: the projection initially emitted raw readable companions, which could make sidecar matching fail for values with tabs or repeated spaces. The final implementation emits compact readable companions and keeps raw sidecar values, with regression coverage for that shape.

## Follow-Ups

The next high-value row-value slices are queue-dispatch artifact/reference rows and remaining worker dispatch retry/reference rows. They are still parser-consumed `key=value` rows and should get adjacent sidecars before they become broader executable surfaces.
