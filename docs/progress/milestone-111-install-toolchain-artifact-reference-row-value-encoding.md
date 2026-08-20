# Milestone 111 - Install Toolchain Artifact Reference Row-Value Encoding

## Summary

Milestone 111 hardens parser-consumed install/toolchain rows:

- `dispatch artifact:`
- `execution artifact:`
- `execution result ...:`
- `install orchestration artifact:`
- `retry dispatch artifact:`
- `retry execution artifact:`
- `retry execution result ...:`
- `retry execution result refs ...:`
- `retry execution result refs summary ...:`

The rows now keep readable compact companions and add RawURL base64 sidecars for artifact paths, dispatch paths, orchestration execution paths, retry-dispatch paths, and source execution references consumed by RuntimeAction and Inspector metadata.

## Why

Install/toolchain artifact rows feed TUI double-confirm install dispatch/retry gates and read-only Inspector details. Before this milestone, leading artifact paths and install/retry reference fields were still parsed from whitespace-delimited row values. Paths containing spaces, tabs, repeated whitespace, or embedded tokens such as `status=`, `mode=`, `dispatch=`, `retry_dispatch=`, `source_execution=`, or `execution=` could truncate values or retarget parsed Toolchain Health metadata.

## Changes

- Added `artifact_b64=` to install dispatch, execution, orchestration, retry-dispatch, and retry-execution artifact rows.
- Added `dispatch_b64=` to install execution artifact/result rows, orchestration rows, retry execution result rows, retry result refs rows, and retry result refs summary rows.
- Added `execution_b64=` to install orchestration artifact rows.
- Added `retry_dispatch_b64=` to install orchestration, retry execution artifact/result, retry result refs, and retry result refs summary rows.
- Added `source_execution_b64=` to retry execution result, retry result refs, and retry result refs summary rows.
- Updated Runtime Tabs install/toolchain parsers to decode leading artifact/reference sidecars and parse machine metadata from rows with legacy readable reference fields stripped first.
- Added `source_execution_b64` to the Runtime Tabs row-field boundary list.
- Preserved old no-sidecar fallback behavior, including legacy rows whose `output` or `error` text contains fake reference-looking fields.

## Validation

Passed before documentation synchronization:

- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestToolchainInstall(Dispatch|Execution|Orchestration|Retry)ProjectionIncludesEncodedReferenceFields' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeInstall(Dispatch|Execution|Orchestration|Retry).*EncodedReferenceFields|TestRuntimeRetryExecutionResultRefs(UsesEncodedReferenceFields|SummaryUsesEncodedReferenceFields)' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'Install|Toolchain|RuntimeAction|RuntimeTabs|RootCommandIncludesProductionCommands' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh scripts/build.sh scripts/install.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Review

Two read-only subagent audits from the prior milestone handoff identified matching producer and parser gaps for install dispatch/execution/orchestration/retry artifact rows and retry result refs rows. Local RED/GREEN coverage verified the same row shapes. A related regression pass caught a legacy fallback risk where retry `output` or `error` text could still retarget no-sidecar reference fields; Runtime Tabs now falls back through the stripped machine row before consulting raw readable text for those references.

## Follow-Ups

Remaining row-value targets are lower-priority parser-consumed managed-bundle and release-managed path references, plus broader display/message rows if a RuntimeAction parser begins consuming them.

No release smoke was rerun before this documentation update because release packaging and bundled manifests were not touched.
