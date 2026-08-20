# Milestone 100 - Worker Queue Claim Artifact Row-Value Encoding

## Summary

Milestone 100 hardens executable `worker queue claim artifact:` Runtime/TUI rows.

The row now carries adjacent RawURL base64 sidecars:

- `artifact_b64`
- `worker_b64`
- `run_b64`
- `execution_b64`

Runtime Tabs decodes the leading artifact path sidecar before action metadata is read, then strips readable worker/run/execution text before parsing claim `status` and `mode`. Worker, run, and execution identifiers are decoded for RuntimeAction/Inspector details.

## Why

`worker queue claim artifact:` rows are executable from the TUI. Their parsed artifact path is passed to queue claim dispatch, cancel, expire, and renew actions. Before this milestone, the parser split the leading artifact path at the first space, and identity fields were read with whitespace-delimited `key=value` parsing.

That meant a claim path or identity value containing text such as `status=failed`, `mode=execute`, `worker=evil`, or `execution=evil` could truncate the artifact path or retarget action metadata.

## Changes

- Added `artifact_b64`, `worker_b64`, `run_b64`, and `execution_b64` to `worker queue claim artifact:` rows in `internal/app/tui_snapshot.go`.
- Added `runtimeLeadingDecodedTextField` in `internal/tui/runtime_tabs.go` for positional leading values with adjacent sidecars.
- Updated `runtimeWorkerQueueClaimArtifactActionForRow` to decode the artifact path sidecar, parse status/mode from identity-stripped rows, and decode worker/run/execution sidecars.
- Added `artifact_b64` to the generic Runtime Tabs field boundary list.
- Added parser and projection regressions for path and identity-field injection.

## Validation

Passed before documentation:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorkerQueueClaimArtifactUsesEncodedReferenceFields' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRuntimeWorkerQueueClaimProjectionIncludesEncodedReferenceFields' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorkerQueueClaimArtifact|TestModelUpdate(Executes|Cancels|Expires|Renews)WorkerQueueClaimArtifact' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRuntimeWorkerQueueClaimProjection|TestNewTUIModelForWorkspace(ShowsRuntimeWorkerQueueClaims|CancelsRuntimeWorkerQueueClaim|ExpiresRuntimeWorkerQueueClaim|ShowsRenewedRuntimeWorkerQueueClaim|ShowsRuntimeWorkerQueueDispatch|ExecutesRuntimeWorkerQueueDispatch)' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`

The full test run uses `GOTMPDIR=/home/zss/t` because `/tmp` had only about 109M free during validation and Go linker temp files exceeded that. A long workspace `GOTMPDIR` was avoided because it changes `t.TempDir()` paths enough to trigger existing TUI width assumptions unrelated to this milestone.

Final strict doctor/build/static checks are recorded in `progress.md`.

## Follow-Ups

The next high-value slice is `worker transport request:`. Its parsed request path and worker id are required by the TUI ack/run executor, and it still lacks sidecars for the leading request path plus `dispatch`, `claim`, `worker`, and `execution` reference fields.
