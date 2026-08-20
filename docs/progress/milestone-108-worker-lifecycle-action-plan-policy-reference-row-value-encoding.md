# Milestone 108 - Worker Lifecycle Action-Plan/Policy Reference Row-Value Encoding

## Summary

Milestone 108 hardens parser-consumed worker lifecycle action-plan and policy rows:

- `worker lifecycle action plan artifact:`
- `worker lifecycle action plan review:`
- `worker lifecycle action plan selection:`
- `worker lifecycle policy artifact:`
- `worker lifecycle policy selection:`
- `worker lifecycle policy run artifact:`
- `worker lifecycle policy scheduler run artifact:`

The rows now keep readable companions and add RawURL base64 sidecars for leading artifact paths, action-plan/policy/selection/review references, actor identity text, timestamps, and scheduler basename metadata.

## Why

Worker lifecycle rows feed RuntimeAction and Inspector metadata. Before this milestone, several action-plan and policy references were still parsed from whitespace-delimited row values after only the earlier free-text sidecars had been added. Paths or operator text containing spaces, tabs, repeated whitespace, or embedded tokens such as `status=`, `mode=`, `plan=`, `policy=`, `selection=`, `review=`, `selected_by=`, `reviewer=`, or fake sidecar-looking text could truncate values or retarget parsed worker lifecycle metadata.

## Changes

- Added `artifact_b64=`, `review_b64=`, `reviewer_b64=`, and `reviewed_at_b64=` to `worker lifecycle action plan artifact:` rows while preserving `review_reason_b64=`.
- Added `reviewer_b64=`, `reviewed_at_b64=`, `plan_b64=`, and `review_b64=` to `worker lifecycle action plan review:` rows while preserving `review_reason_b64=`.
- Added `selected_by_b64=`, `selected_at_b64=`, `cleared_by_b64=`, `cleared_at_b64=`, `plan_b64=`, `selection_b64=`, and `review_b64=` to `worker lifecycle action plan selection:` rows while preserving `reason_b64=`.
- Added leading `artifact_b64=` to `worker lifecycle policy artifact:` rows.
- Added `policy_b64=`, `selection_b64=`, `selected_by_b64=`, `selected_at_b64=`, `cleared_by_b64=`, and `cleared_at_b64=` to `worker lifecycle policy selection:` rows while preserving `reason_b64=`.
- Added `artifact_b64=`, `policy_b64=`, and `selection_b64=` to `worker lifecycle policy run artifact:` rows while preserving `skip_b64=`.
- Added `artifact_b64=`, `policy_b64=`, `selection_b64=`, `policy_basename_b64=`, and `selection_basename_b64=` to `worker lifecycle policy scheduler run artifact:` rows while preserving `skip_b64=` and `error_b64=`.
- Updated Runtime Tabs action-plan and policy parsers to decode leading artifact sidecars and parse machine metadata from rows with legacy readable reference, actor, timestamp, skip, error, reason, and review text stripped first.
- Added `plan_b64`, `policy_b64`, `policy_basename_b64`, and `selection_basename_b64` to the Runtime Tabs row-field boundary list.
- Kept old no-sidecar fallback behavior for legacy worker lifecycle action-plan and policy rows.

## Validation

Passed before documentation synchronization:

- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorkerLifecycle(ActionPlanArtifactUsesEncodedReferenceFields|ActionPlanSelectionUsesEncodedReferenceFields|PolicyArtifactUsesEncodedReferenceFields|PolicySelectionUsesEncodedReferenceFields|PolicyRunArtifactUsesEncodedReferenceFields|PolicySchedulerRunArtifactUsesEncodedReferenceFields)' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'WorkerLifecycleActionPlan|WorkerLifecyclePolicy|NewTUIModelForWorkspaceShowsWorkerLifecycle|NewTUIModelForWorkspace(Selects|Clears).*WorkerLifecycle' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'WorkerLifecycle(ActionPlan|Policy)|RuntimeTabs|RuntimeAction|RootCommandIncludesProductionCommands' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh scripts/build.sh scripts/install.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go internal/app/worker_queue_claims_test.go`

## Review

Two read-only subagents audited the action-plan and policy row scope before documentation synchronization. Both identified the same missing action-plan/policy artifact, selection, review, run, scheduler, actor, timestamp, and basename sidecars. Local RED/GREEN tests covered those parser and producer gaps, including rows where readable text contains fake sidecar-looking or machine-field tokens.

## Follow-Ups

The next row-value targets are worker placement rows, worker lease recovery rows, and install/toolchain artifact rows. Broader display/message rows remain lower priority unless a RuntimeAction parser consumes them for executable or Inspector metadata.

No release smoke was rerun because release packaging and bundled manifests were not touched.
