# Milestone 87: Replay Workbench Selection/Presence Row-Value Encoding

## Summary

Milestone 87 extends the runtime row-value sidecar pattern to replay workbench selection and presence rows.

Rows still keep their readable `reason=` field for TUI scanning, but now also emit `reason_b64=` using RawURL base64:

- `replay workbench session selection:`
- `replay workbench collaboration selection:`
- `replay workbench collaboration note selection:`
- `replay workbench collaboration presence artifact:`

Runtime Tabs action parsing now removes the legacy `reason` segment before extracting machine fields, then decodes `reason_b64` for Inspector/action text. This prevents free-text reason strings containing tokens such as `session=`, `primary=`, `selected=`, `path=`, `collaboration=`, or `session_path=` from retargeting selection/presence metadata.

## Boundaries

This milestone does not change replay workbench write gates, browser mutation gates, TUI double-confirm behavior, provider calls, worker dispatch, target contact, or replay event mutation. It is a row-format compatibility hardening only.

## Validation

Focused parser validation passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeReplayWorkbench(CollaborationPresenceUsesEncodedReasonText|CollaborationNoteSelectionUsesEncodedReasonText|CollaborationSelectionUsesEncodedReasonText|SessionSelectionUsesEncodedReasonText)' -count=1 -v
```

Focused app projection validation passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceShowsReplayWorkbench(CollaborationPresence|CollaborationNotes|CollaborationSelection|SessionSelection)' -count=1 -v
```

Related replay workbench regression passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -run 'ReplayWorkbench|replay workbench|RuntimeTabs|RuntimeAction' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'ReplayWorkbench|RootCommandIncludesProductionCommands' -count=1 -v
```

## Remaining Work

- Extend row-value sidecars to report projection execution review/current rows.
- Extend row-value sidecars to worker lifecycle action-plan/selection rows.
- Continue replacing unescaped free-text `error`, `message`, and `output` fields with readable fields plus encoded sidecars where RuntimeAction parsing consumes the row.
