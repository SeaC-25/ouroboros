# Milestone 98: Runtime Decoded-Field Fallback Hardening

Milestone 98 hardens the shared Runtime Tabs decoded-field helpers that back the row-value sidecar work from milestones 85 through 97.

## Scope

`runtimeDecodedRowField` and `runtimeDecodedRowFieldWithFallbackRow` now trust base64 sidecars only when one of these conditions holds:

- The row has a readable `<key>=` value followed by an adjacent `<key>_b64=` sidecar whose decoded text matches the readable text.
- The row is sidecar-only for that key, with no readable `<key>=` value in the row and no fallback readable value.

Rows with readable or fallback values now ignore mismatched map-level fake `_b64` tokens. The sidecar matcher also accepts older underscore-compacted readable text, for example `reason=current_reviewed_action_plan` with a sidecar that decodes to `current reviewed action plan`.

## Compatibility

Sidecar-only model generation preview rows still decode `preview_b64` / `stream_preview_b64`.

Readable-plus-sidecar rows that use space-compacted text still decode as before. Readable-plus-sidecar rows that use underscore-compacted text now also decode the original raw text rather than falling back to the compact display value.

This is only Runtime Tabs parser hardening. It does not create artifacts, execute tools, dispatch workers, contact providers, contact endpoints/targets, write reports, or change any existing execution gate.

## Validation

RED tests first reproduced the old behavior:

- A readable `source=` value could be overridden by a mismatched but decodable `source_b64=`.
- A fallback `command=` value could be overridden by a mismatched but decodable `command_b64=`.
- Sidecar-only rows still needed to decode.
- The first GREEN attempt exposed historical underscore-compacted readable rows, so a focused regression was added before expanding the matcher.

Focused and related validation passed before final repository validation:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeDecodedRowField(IgnoresMismatchedMapSidecarWhenReadableValueExists|WithFallbackRowIgnoresMismatchedMapSidecarWhenFallbackValueExists|KeepsSidecarOnlyCompatibility|AcceptsUnderscoreCompactedReadableSidecar)' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceShows(WorkerLifecycleActionPlanReviewSelectionRows|ReportProjectionExecutionReviewSelectionRows)' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app -run 'RuntimeDecodedRowField|UsesEncoded|IgnoresFake|Sidecar|RowValue|ManagedBundle|ReleaseManaged|InstallExecution|RetryExecution|RunModelGeneration|ModelGenerationArtifact|ModelToolPlanPreview|ReplayWorkbench|ReportProjection|WorkerLifecycle' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`

Euclid's read-only scan confirmed the risk and the intended minimal rule. Hume's quality review approved the final helper hardening with no blockers and separately ran a focused TUI validation.

Final repository validation passed after documentation synchronization:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Follow-Ups

- Add worker identity/path sidecars where parser-consumed values can contain row delimiters.
- Continue broader `message` and remaining display-only text row-value encoding.
