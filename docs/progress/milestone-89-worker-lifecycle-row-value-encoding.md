# Milestone 89: Worker Lifecycle Row-Value Encoding

Milestone 89 extends parser-safe row-value sidecars to worker lifecycle RuntimeAction rows that still carried unescaped free text.

## Scope

Workspace-bound projections now keep the legacy readable fields and add RawURL base64 sidecars for parser-safe recovery:

- `worker lifecycle policy selection:` uses `reason_b64`.
- `worker lifecycle action plan selection:` uses `reason_b64`.
- `worker lifecycle action plan artifact:` and its compact review rows use `review_reason_b64`.
- `worker lifecycle policy run artifact:` uses `skip_b64`.
- `worker lifecycle policy scheduler run artifact:` uses `skip_b64` and `error_b64`.
- `worker dispatch retry scheduler run artifact:` uses `skip_b64` and `error_b64`.

Runtime Tabs now strips those legacy free-text fields before reading machine metadata and decodes the sidecars for Inspector/action text. This prevents embedded `policy=`, `selection=`, `plan=`, `review=`, `current=`, `actions=`, `error=`, or retry-transport tokens from retargeting action metadata.

A review pass found one must-fix after the initial GREEN: the legacy-field stripper used the first matching same-name sidecar marker as the text boundary, so a readable free-text value containing fake `reason_b64=` or `skip_b64=` could leave later fake machine fields in the parser input. The boundary now uses the last same-name sidecar marker as the readable legacy text terminator. Regression tests cover fake `reason_b64`, `skip_b64`, and skip/error sidecar marker text.

## Compatibility

The old readable fields remain in every row so existing CLI/TUI scanning output stays usable. Legacy rows without sidecars still fall back to the old `key=` parsing path.

This is only a row-format compatibility layer. It does not add execution shortcuts, request paths, scheduler behavior, worker dispatch, target contact, provider calls, replay mutation, or artifact writes beyond the pre-existing projection rows.

## Validation

RED parser tests first reproduced field contamination and truncation for worker lifecycle selection/review/skip/error rows. GREEN then added sidecar emission and parser isolation.

Focused and related validation run so far:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'IgnoresFake.*SidecarMarkers|IgnoresFakeSkipSidecarMarkers' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'IgnoresFake.*SidecarMarkers|IgnoresFakeSkipSidecarMarkers|UsesEncoded.*Text|UsesEncoded.*Skip' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorkerLifecycle(PolicySelectionUsesEncodedReasonText|ActionPlanSelectionUsesEncodedReasonText|ActionPlanArtifactUsesEncodedReviewReasonText)' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestWorkerLifecycle(ActionPlanProjectionIncludesEncodedReviewText|PolicySelectionProjectionIncludesEncodedReasonText)' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeWorker(LifecyclePolicyRunArtifactUsesEncodedSkipText|LifecyclePolicySchedulerRunArtifactUsesEncodedSkipErrorText|DispatchRetrySchedulerRunArtifactUsesEncodedSkipErrorText)' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestWorker(LifecyclePolicyRunProjectionIncludesEncodedSkipText|LifecyclePolicySchedulerRunProjectionIncludesEncodedSkipErrorText|DispatchRetrySchedulerRunProjectionIncludesEncodedSkipErrorText)' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -run 'WorkerLifecycle|workerLifecycle|PolicyScheduler|WorkerDispatchRetryScheduler|DispatchRetryScheduler|RuntimeWorker' -count=1 -v`

Projection tests now bind `skip_b64` and `error_b64` assertions to the specific `... artifact:` rows and assert display-only `... skip:` / `... error:` rows do not carry `_b64=` sidecars.

Final validation after the review fix:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

All final validation commands passed. No release smoke was rerun because release packaging and bundled manifests were not touched in this row-format slice.

## Follow-Ups

- Add sidecars for worker lifecycle audit item `reason` and `worker lifecycle action:` `reason`.
- Add sidecars for `worker dispatch retry execution result:` `reason`.
- Decide whether display-only skip/error detail rows should get sidecars or move to a structured row model.
- Continue broader `error`, `output`, and `message` row-value encoding.
