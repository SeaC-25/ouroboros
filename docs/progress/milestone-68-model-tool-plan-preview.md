# Milestone 68: Model Tool-Plan Preview

Date: 2026-06-05

## Summary

Milestone 68 adds a constrained bridge from explicit run-integrated provider generation to a tool-plan review preview.

The default run path is unchanged. The preview is available only when an operator enables both run-integrated provider generation and `--provider-generate-tool-plan-preview`, or when app callers set the matching `RunOptions` field.

The scanner accepts only one-line markers:

- `ouro_action: <semantic_action>`
- `semantic_action: <semantic_action>`

The action must be declared by the compiler. A valid marker creates a metadata-only tool compile preview review, a `model_tool_plan_preview` replay event, and a direct JSON artifact under `workspace/runtime/model-tool-plan-previews/`. Missing, malformed, or unsupported markers write only a skipped replay event and create no review or artifact.

## Surfaces

- App:
  - `RunOptions` accepts `ProviderGenerationToolPlanPreview`.
  - `ExtractModelToolPlanPreview` extracts marker/action/validity metadata plus the model-output digest.
  - `RecordModelToolPlanPreviewArtifact`, `ListModelToolPlanPreviewArtifacts`, and `ReadModelToolPlanPreviewArtifact` manage workspace-confined metadata-only artifacts.
  - The closed-loop planner generation path creates a compile-preview review only for valid declared actions.
- Replay:
  - `model_tool_plan_preview` events project into Model Runtime preview state.
  - Preview events do not enter token/cost usage aggregation.
- CLI:
  - `ouro run <workspace> --provider-generate --provider-generate-tool-plan-preview` enables marker extraction after a successful provider generation.
  - `ouro provider tool-plan-previews <workspace> [artifact]` lists or inspects persisted preview artifacts read-only.
- TUI:
  - Replay-derived `model tool plan preview:` rows render marker/action/validity metadata.
  - Workspace-bound Model Runtime renders persisted `model tool plan preview:` artifact rows.
  - Runtime Tabs Inspector details are read-only and reject request/execution shortcuts.

## Boundary

- The preview persists marker preview, action, validity, reason, model-output digest, generation artifact path, compile-preview review path/counts, artifact digest, and explicit side-effect flags only.
- Prompt text, complete assistant text, raw stream chunks, bearer tokens, provider request/response bodies, raw provider errors, and executable model instructions must not be persisted.
- Invalid, missing, or malformed markers write only skipped replay metadata and no review/artifact.
- This does not select current compile-preview markers, execute tools, dispatch workers, run another tool round, contact endpoints, or contact scoped targets.

## Validation

RED was confirmed before implementation with:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'TestProviderToolPlanPreviewsCommandListsAndReadsArtifacts' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'ModelToolPlanPreview|ToolPlanPreview|ProviderGeneration|NewTUIModelForWorkspaceShowsModelToolPlanPreviewArtifacts' -count=1`

Focused GREEN and touched-package regression passed with:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/eventlog ./internal/tui ./cmd/ouro -run 'ToolPlanPreview|ToolPlanPreviews|ProviderGeneration|ModelGeneration|ModelUsagePreviews|RuntimeTabs|ModelRuntime|RootCommand' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/eventlog ./internal/tui ./cmd/ouro -count=1`

Full validation after documentation synchronization passed with:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `git diff --check`
- `gofmt -l cmd/ouro/main.go cmd/ouro/main_test.go internal/app/run.go internal/app/planner_loop.go internal/app/model_tool_plan_preview_artifact.go internal/app/run_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/eventlog/replay.go internal/eventlog/replay_test.go internal/tui/replay.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

Local static safety review also checked the model tool-plan preview path for execution/dispatch/selection/target-contact calls. The only tool-plan preview app-side mutator beyond event/artifact writes is `CreateToolCompilePreviewReview` for a valid declared action; execution dispatch, current selection, worker dispatch, and target-contact calls are outside this path.
