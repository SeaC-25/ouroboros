# Milestone 90: Worker Runtime Reason Row-Value Encoding

Milestone 90 closes the next worker-runtime row-value gap by adding parser-safe sidecars for three remaining `reason` fields that feed Runtime Tabs actions.

## Scope

Workspace-bound projections now keep the legacy readable `reason=` fields and add RawURL base64 `reason_b64=` sidecars to:

- `worker lifecycle audit item:`
- `worker lifecycle action:`
- `worker dispatch retry execution result:`

Runtime Tabs now strips the legacy readable `reason` text before reading machine metadata for those rows and decodes `reason_b64` for Inspector/action text. This prevents embedded `request_path=`, `dispatch=`, `actions=`, `skipped_unsafe=`, `failed=`, `transport=`, `worker=`, `run=`, or `status=` tokens from retargeting action metadata or aggregate counters.

## Compatibility

The readable `reason=` fields remain in each row for existing human-readable output. Legacy rows without `reason_b64` still fall back to the old field parser.

This is only a row-format compatibility layer. It does not add execution shortcuts, request paths, scheduler behavior, worker dispatch, target contact, provider calls, replay mutation, or artifact writes beyond the pre-existing projection rows.

## Validation

RED parser tests first reproduced reason truncation and machine-field contamination for all three row families. GREEN added sidecar emission and parser isolation.

The quality-review subagent also found an adjacent row-value regression in the earlier `compile-preview review artifact:` parser: the production row already emitted both `reason=` and `reason_b64=`, but the parser still read machine fields such as `reviewed_at` from the unstripped row. A RED test now uses the real readable-plus-sidecar row shape, and the parser reads compile-preview review machine metadata from a legacy-reason-stripped row while still decoding `reason_b64` for display text.

Focused and related validation run so far:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app -run 'TestRuntimeWorkerLifecycle(AuditItemUsesEncodedReasonText|ActionUsesEncodedReasonText)|TestRuntimeWorkerDispatchRetryExecutionResultUsesEncodedReasonText|TestWorkerLifecycle(ActionProjectionIncludesEncodedReasonText|AuditItemProjectionIncludesEncodedReasonText)|TestWorkerDispatchRetryExecutionResultProjectionIncludesEncodedReasonText' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -run 'WorkerLifecycle|workerLifecycle|WorkerDispatchRetry|DispatchRetry|RuntimeWorker' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run TestRuntimeCompilePreviewReviewArtifactUsesEncodedReasonText -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeCompilePreviewReviewArtifact(UsesEncodedReasonText|IgnoresPathInReasonText)|CompilePreviewReview' -count=1 -v`

Final validation after the quality-review fix:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

All final validation commands passed. No release smoke was rerun because release packaging and bundled manifests were not touched in this row-format slice.

## Follow-Ups

- Continue broader row-value encoding for remaining `error`, `output`, `message`, and display-only text rows.
- Decide whether row formatting should move from ad hoc readable strings plus sidecars to a structured projection model.
