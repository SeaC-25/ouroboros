# Milestone 85: Runtime Row-Value Encoding Seed

Date: 2026-06-05

## Summary

Runtime `key=value` rows now have a first parser-safe free-text sidecar pattern.

Workspace-bound app projections keep the existing human-readable fields:

- `preview=...`
- `stream_preview=...`
- `reason=...`

They also emit RawURL base64 sidecars where those values can contain spaces or embedded `key=value` text:

- `preview_b64`
- `stream_preview_b64`
- `reason_b64`

Runtime Tabs prefer the encoded sidecar when building Inspector/action metadata and fall back to the legacy field when the sidecar is absent or invalid.

## Covered Rows

- `run model generation:`
- `model generation artifact:`
- `model tool plan preview:`
- `model tool plan preview acceptance artifact:`
- `compile-preview review artifact:`

This preserves existing readable rows while preventing whitespace-bearing preview/reason text, or embedded strings such as `path=`, `mode=`, `validity=`, `digest=`, and `reviewed_at=`, from changing the metadata parsed for those actions.

## Safety Boundary

This milestone changes row encoding and parsing only.

It does not:

- Execute model output.
- Add provider calls.
- Accept previews automatically.
- Create compile-preview execution dispatches.
- Dispatch workers or tools.
- Contact endpoints or targets.
- Mutate replay/report state beyond existing explicit gates.

## Validation

RED tests first showed that encoded preview/reason fields were ignored and app projections did not emit them. Focused validation then passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntime(ModelToolPlanPreviewUsesEncodedPreviewText|CompilePreviewReviewArtifactUsesEncodedReasonText|RunModelGenerationUsesEncodedPreviewText|ModelGenerationArtifactUsesEncodedPreviewText)' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceShows(ModelGenerationReviewArtifacts|ModelToolPlanPreviewArtifacts|CompilePreviewReviewArtifacts)' -count=1 -v
```

Related regression also passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -run 'ModelGeneration|ModelToolPlanPreview|CompilePreviewReview|RuntimeAction|RuntimeTabs' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'ToolPlanPreview|ModelGeneration|RootCommandIncludesProductionCommands' -count=1 -v
```

Full validation passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build
sh -n scripts/release.sh
git diff --check
gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go
```

## Remaining Work

- Extend the same sidecar pattern to replay workbench session/collaboration/note rows.
- Extend it to report projection execution review/current rows.
- Extend it to worker lifecycle action-plan/selection rows.
- Add encoded `error`, `message`, `output`, `body`, and `review_reason` fields where runtime rows still contain unescaped free text.
- Consider a shared row-format helper package once more producers and consumers use the pattern.
