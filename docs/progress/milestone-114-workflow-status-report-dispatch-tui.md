# Milestone 114: Workflow Status Report Dispatch TUI Creation

Date: 2026-06-07

## Summary

Milestone 114 closes one more operator workflow gap in the TUI. A workspace-bound `workflow status:` row can now create the next report projection dispatch directly from Model Runtime, but only for the narrow state where:

- `stage=report-projection`
- `next_gate=create-report-projection-dispatch`
- `report_run` is present
- workflow status is `ready`

The action uses the existing two-step `x` confirmation. The first `x` arms confirmation and writes nothing. The second `x` calls `CreateReportProjectionDispatch` with `actor=tui` and `reason=created from TUI workflow status`, writes one dry-run metadata dispatch artifact, switches to Timeline Lanes, and selects the new `report projection dispatch artifact:` row.

## Boundaries

This is dispatch creation only. It does not call `ExecuteReportProjectionDispatch`, does not write Markdown reports, does not append `report_ready` or any replay event, does not write report dispatch execution audits, and does not contact providers, targets, endpoints, workers, leases, tool runners, container runtime, or transport outboxes.

All other `workflow-status` rows remain unsupported/read-only through `x`, including preview acceptance, safety authorization, compile execution, report dispatch execution, report review, and provider-preview setup.

The `workflow-status` CLI command remains read-only.

## Implementation

- `internal/tui/runtime_tabs.go`
  - Treats `workflow-status` as metadata-only executable only when the report-projection dispatch creation gate is fully bound.
  - Keeps every other workflow-status row read-only.

- `internal/tui/app.go`
  - Adds `workflow-status` to the double-confirm execution whitelist.
  - Rejects read-only workflow-status actions before executor invocation.
  - Routes workflow report-dispatch creation results to Timeline Lanes and selects the created dispatch artifact row.

- `internal/app/tui_snapshot.go`
  - Adds a workspace-bound executor branch for the narrow workflow-status report dispatch gate.
  - Calls `CreateReportProjectionDispatch` only, then returns report dispatch runtime rows.

- `internal/app/tui_snapshot_test.go`
  - Adds an end-to-end TUI test proving first `x` writes nothing and second `x` creates exactly one dry-run dispatch without report execution, Markdown, replay mutation, worker queue claims, or transport requests.

## Validation

Focused validation passed before this document:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceCreatesReportProjectionDispatchFromWorkflowStatusAfterDoubleConfirm -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'WorkflowStatus|ReportProjection|RuntimeAction|RuntimeTabs|DoesNotExecuteWorkflowStatusRows' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'WorkflowStatus|ReportProjectionDispatch|NewTUIModelForWorkspace.*ReportProjection|NewTUIModelForWorkspace.*WorkflowStatus' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'WorkflowStatus|ReportProject|RootCommandIncludesProductionCommands' -count=1 -v
```

Final validation also passed:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
sh -n scripts/release.sh scripts/build.sh scripts/install.sh
git diff --check
gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/app.go internal/tui/runtime_tabs.go
```

No release smoke was rerun because release packaging and bundled manifests were not changed.
