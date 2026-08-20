# Milestone 88: Report Projection Row-Value Encoding

## Summary

Milestone 88 extends parser-safe row-value sidecars to report projection action rows:

- `report projection dispatch artifact:`
- `report projection dispatch execution artifact:`
- `report projection execution review:`
- `report projection execution selection:`

Rows still keep readable `reason=` and `review_reason=` fields for TUI scanning, but workspace-bound projections now emit RawURL base64 `reason_b64=` and `review_reason_b64=` sidecars where applicable. Runtime Tabs strips legacy reason/review text before parsing machine fields and decodes the sidecars for Inspector/action metadata.

This prevents free-text values containing tokens such as `execution=`, `dispatch=`, `report=`, `planned_report=`, `run=`, `decision=`, `reviewed_at=`, or `current=` from retargeting report projection dispatch, execution, review, or current-selection metadata.

## Boundaries

This milestone does not change report projection execution gates, report selection semantics, browser mutation gates, TUI double-confirm behavior, provider calls, worker dispatch, target contact, replay-event mutation rules, or Markdown report write safety. It is a row-format compatibility hardening only.

## Validation

Focused parser validation passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeReportProjection(DispatchArtifactUsesEncodedReasonText|DispatchExecutionArtifactUsesEncodedReviewText|ExecutionReviewUsesEncodedReasonText|ExecutionSelectionUsesEncodedReasonText)' -count=1 -v
```

Focused app projection validation passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceShowsReportProjection(DispatchRows|ExecutionRows|ExecutionReviewSelectionRows)' -count=1 -v
```

Related report projection regression passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'ReportProjection|RootCommandIncludesProductionCommands' -count=1 -v
```

## Remaining Work

- Extend `reason_b64` / `review_reason_b64` to worker lifecycle policy/action-plan selection and action-plan artifact rows.
- Add `skip_b64` and `error_b64` for worker lifecycle scheduler/policy rows that expose free-text skip/error metadata.
- Continue replacing unescaped free-text `error`, `message`, `output`, and result text where RuntimeAction parsing consumes the row.
