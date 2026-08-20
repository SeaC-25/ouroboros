# Milestone 99 - Worker Lifecycle Action Identity Row-Value Encoding

## Summary

Milestone 99 hardens `worker lifecycle action:` Runtime/TUI rows where worker, run, and execution identifiers are rendered as readable `key=value` fields but can contain spaces or machine-looking tokens.

The row now carries adjacent RawURL base64 sidecars:

- `worker_b64`
- `run_b64`
- `execution_b64`

Runtime Tabs strips the readable identity fields before reading action metadata such as `status`, `mode`, `request_path`, `dispatch`, and `recommended`, then decodes the sidecars for Inspector/action details. Existing `reason_b64` behavior remains unchanged.

## Why

`RegisterRuntimeWorker` trims worker ids but does not reject spaces or `key=value` text. Worker lifecycle action plans can therefore project rows where a worker id, run id, or execution id contains text such as `status=failed`, `mode=execute`, `request_path=/tmp/evil.json`, or `recommended=skip`.

Before this milestone, `runtimeWorkerLifecycleActionForRow` read machine fields from the unstripped row. A readable identity value could truncate the displayed identity and retarget action metadata.

## Changes

- Added `worker_b64`, `run_b64`, and `execution_b64` to `worker lifecycle action:` rows in `internal/app/tui_snapshot.go`.
- Updated `runtimeWorkerLifecycleActionForRow` in `internal/tui/runtime_tabs.go` to strip `reason`, `worker`, `run`, and `execution` readable text before machine-field parsing.
- Decoded worker/run/execution sidecars for RuntimeAction details.
- Added the new sidecar keys to the generic Runtime Tabs row-field boundary list.
- Added parser and projection regressions for identity-field injection.

## Validation

Passed before documentation:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorkerLifecycleActionUsesEncodedIdentityText' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestWorkerLifecycleActionProjectionIncludesEncodedIdentityText' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorkerLifecycleAction|TestRuntimeDecodedRowField|TestRuntimeWorkerLifecycleAuditItemAction|TestModelUpdateDoesNotExecuteWorkerLifecycleActionRows' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestWorkerLifecycleAction|TestNewTUIModelForWorkspaceShowsWorkerLifecycleAction|TestNewTUIModelForWorkspaceSelectsWorkerLifecycleActionPlan|TestNewTUIModelForWorkspaceRejectsUnselectableWorkerLifecycleActionPlan|TestNewTUIModelForWorkspaceClearsWorkerLifecycleActionPlan' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`

Final strict doctor/build/static checks are recorded in `progress.md`.

## Follow-Ups

The next row-value slices should cover worker transport, queue, dispatch, and path/reference fields that still rely on whitespace-delimited row parsing:

- `worker transport request:`
- `worker queue claim artifact:`
- worker queue dispatch artifact/result rows
- `worker dispatch retry execution result:` reference fields

Broader message/display-only text rows remain a later audit target unless a RuntimeAction parser starts consuming them.
