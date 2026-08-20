# Milestone 86: Replay Workbench Row-Value Encoding

Date: 2026-06-05

## Summary

Replay workbench artifact rows now use the same parser-safe free-text sidecar pattern introduced in Milestone 85.

Workspace-bound app projections keep the existing readable fields:

- `reason=...`
- `review_reason=...`
- `body=...`

They also emit RawURL base64 sidecars where those values can carry spaces or embedded `key=value` text:

- `reason_b64`
- `review_reason_b64`
- `body_b64`

Runtime Tabs prefer those encoded sidecars for Inspector/action metadata and fall back to the legacy fields when the sidecar is absent or invalid.

The artifact action parsers now also strip legacy `reason`, `review_reason`, and note `body` text from the machine-field parsing view before reading `path`, `current`, `collaboration`, and `session_path`. The original row remains available for encoded text decoding and display fallback. `runtimeRowField` also treats the new sidecar keys as field boundaries so empty sidecars do not leak into legacy fallback text.

## Covered Rows

- `replay workbench session artifact:`
- `replay workbench collaboration artifact:`
- `replay workbench collaboration note artifact:`

This prevents free text containing strings such as `current=`, `path=`, `collaboration=`, or `session_path=` from contaminating artifact action details while retaining the existing readable row format.

## Safety Boundary

This milestone changes row encoding and parsing only.

It does not:

- Select or clear replay workbench sessions, collaborations, or notes.
- Write browser artifacts.
- Enable hosted workbench POST routes.
- Mutate replay events or reports.
- Contact providers, workers, endpoints, or targets.
- Execute tools.

## Validation

RED tests first showed replay workbench artifact rows did not emit or decode the new sidecars. Focused validation then passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeReplayWorkbench(SessionArtifactUsesEncodedReviewText|SessionArtifactUsesLegacyReasonBeforeEmptySidecar|CollaborationArtifactUsesEncodedReviewText|CollaborationNoteArtifactUsesEncodedText)' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceShowsReplayWorkbench(SessionArtifacts|CollaborationArtifacts|CollaborationNotes)' -count=1 -v
```

Related replay workbench regression also passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -run 'ReplayWorkbench|replay workbench|RuntimeTabs|RuntimeAction' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'ReplayWorkbench|RootCommandIncludesProductionCommands' -count=1 -v
```

A read-only subagent review found no must-fix issues after the parser hardening and regression additions.

Final validation passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build
sh -n scripts/release.sh
git diff --check
gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go
```

## Remaining Work

- Extend sidecars to replay workbench selection/presence rows where free-text reasons still appear.
- Extend sidecars to report projection execution review/current rows.
- Extend sidecars to worker lifecycle action-plan/selection rows.
- Add encoded `error`, `message`, and `output` fields for runtime rows that still carry unescaped free text.
