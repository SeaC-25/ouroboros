# Milestone 57: Planner-Selected Compile Preview

## Implemented

- Extended the `closed-loop-planner-preview` stage so it reads the Sisyphus-selected intent's semantic action from the original intent payload.
- Added the run-level semantic action to `intent-production-run`, so the planner-selected run intent can be compiled without falling back to an unrelated default.
- Created a dry-run metadata-only tool compile preview review under `workspace/toolchain/compile-preview-reviews/` during the planner preview stage.
- Wrote a current metadata-only compile preview review selection marker at `workspace/toolchain/compile-preview-review-selection.json` with `selected_by=closed-loop-planner`.
- Added selected action, compile-preview mode, review path, selection path, node count, and warning count to the durable `planner_review` attempt payload.
- Updated the planner suggestion text so replay, the TUI Attack Graph, and Markdown `Next-Run Suggestions` expose the planner-selected compile preview summary.
- Reused workspace-bound TUI Toolchain Health compile-preview rows so operators can inspect the review, current selection, and compiled plan nodes.

## Boundaries

- This is still a metadata-only planner review path.
- It does not execute the selected compile preview plan.
- It does not dispatch workers or tools.
- It does not contact providers, transports, endpoints, scoped targets, or container runtimes.
- It does not call model generation, route providers, stream tool calls, or perform provider failover.
- It does not mutate worker leases, queue claims, report dispatch artifacts, replay workbench artifacts, or model usage ledgers.
- It preserves default warning-only runs under disabled safety boundaries; the planner-internal selection marker does not convert default runs into strict safety-authorization runs.

## Validation

- Baseline full repository validation passed before implementation with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`.
- RED app integration failed as expected with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestRunWorkspaceRecordsPlannerSelectedCompilePreviewReview -count=1 -v`, because no planner-selected compile preview review existed.
- GREEN app integration passed with the same focused test after implementation.
- Focused planner/run/compile-preview regression passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRunWorkspaceRecords(DurableEventsReplayAndReport|PlannerSelectedCompilePreviewReview|ObservableStageAttemptEvents)|TestPreviewToolCompilePlan|TestCreateToolCompilePreviewReview|TestSelectToolCompilePreviewReview|TestNewTUIModelForWorkspaceShowsCompilePreviewReviewSelection' -count=1 -v`.
- Safety-boundary regression passed after fixing the planner-internal selection path with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRunWorkspace(PersistsWorkspaceSafetyAuditEvent|DefaultSafetyAuditKeepsWorkerLeaseWritesWarningOnly|CanDisableCommandTargetPolicyForAuthorizedScope|CanDisableRunCommandURLArgumentPolicy|CanDisableEmbeddedRunCommandURLArgumentPolicy|CanDisableRunCommandCIDRArgumentPolicy|CanDisableRunCommandChallengeArgumentPolicy|CommandRunnerCanDisableIsolationForLegacyTools|RecordsPlannerSelectedCompilePreviewReview)' -count=1 -v`.
- Full app package passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`.
- Replay/report/TUI/CLI focused regression passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/eventlog ./internal/report ./internal/tui ./cmd/ouro -run 'Planner|RunWorkspace|CompilePreview|Replay|Report|RuntimeTabs|RootCommand' -count=1`.
- Full repository validation passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`.
- Read-only code review found one minor future-risk around duplicate `intent_id` payloads. Added RED/GREEN coverage and changed selected-action lookup to use the latest non-empty matching action with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestClosedLoopPlannerSelectedActionUsesLatestMatchingIntentPayload|TestRunWorkspaceRecordsPlannerSelectedCompilePreviewReview' -count=1 -v`.
- Final validation after documentation synchronization passed with `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`, `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict` printing `workspace: ready`, `git diff --check`, and touched-file `gofmt -l internal/app/planner_loop.go internal/app/run.go internal/app/run_test.go`.

## Remaining Work

- Closed-loop execution remains a follow-up: the selected compile preview is not executed until an explicit operator-reviewed execution gate exists.
- Run-integrated model generation/accounting, provider routing/failover, production binary payload bundling, and autonomous distributed worker scheduling beyond bounded/gated audit runs remain outside this milestone.
