# Milestone 71: Workflow Status Safety Gate

## Summary

Added a read-only safety-authorization projection to `workflow-status` so the IDE explains why the next operator gate is blocked instead of failing the status command or suggesting an unusable accept/execute command.

## Implemented

- Added `WorkspaceWorkflowStatus` safety error metadata.
- Added `next_gate=safety-authorization` for:
  - pending model tool-plan preview acceptance blocked by strict safety authorization
  - compile-preview review execution dispatch execution blocked by strict safety authorization
- Kept workflow status selection reads metadata-only without relaxing the public strict `ReadToolCompilePreviewReviewSelection` API.
- Added `eventlog.OpenSQLiteReadOnly` and switched workflow report run-id inspection to that read-only path so status projection does not migrate SQLite or create WAL sidecars.
- Extended CLI output with `safety_errors` and `workflow-safety-error` rows.
- Extended Model Runtime `workflow status:` rows with blocked status and safety error counts.
- Added Runtime Tabs regressions proving ready and blocked `workflow-status` rows reject both request (`r`) and execute (`x`) paths.

## Safety Boundary

This milestone is visibility only. It reads existing preview, acceptance, compile-review selection, dispatch/execution, report, and replay metadata. It may recommend running strict doctor as a command hint, but it does not run doctor, write or renew `authorization.yaml`, accept previews, select or clear markers, create dispatches, execute compile-review nodes, write Markdown reports, call providers, dispatch workers, contact endpoints or targets, read token environment values, probe container runtime, write usage records, or append replay events.

## Validation

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro ./internal/eventlog -run 'WorkflowStatus|OpenSQLiteReadOnly|DoesNotRequestReadOnlyRuntimeActions|DoesNotExecuteWorkflowStatusRows|NewTUIModelForWorkspaceShowsWorkflowStatusSafetyAuthorizationGate' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro ./internal/eventlog -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `git diff --check`
- `gofmt -l internal/app/workflow_status.go internal/app/workflow_status_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/eventlog/sqlite.go internal/eventlog/sqlite_test.go cmd/ouro/main.go cmd/ouro/main_test.go internal/tui/runtime_tabs_test.go`
