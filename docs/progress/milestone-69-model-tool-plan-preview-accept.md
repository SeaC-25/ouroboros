# Milestone 69: Model Tool-Plan Preview Accept

Date: 2026-06-05

## Summary

Milestone 69 adds the explicit accept/select gate for model-generated tool-plan preview artifacts.

Milestone 68 preview artifacts remain metadata-only recommendations. A preview does not become the current compile-preview review until an operator accepts it through this gate.

## Surfaces

- App:
  - `AcceptModelToolPlanPreviewArtifact` accepts only persisted preview artifacts with `status=completed` and `execution_mode=metadata-only`.
  - The linked compile-preview review is re-read and must still be `dry-run` / `metadata-only` with the same semantic action.
  - Acceptance writes `workspace/runtime/model-tool-plan-preview-acceptances/*.json`.
  - Acceptance selects the linked compile-preview review through the existing `SelectToolCompilePreviewReview` path.
- CLI:
  - `ouro provider tool-plan-preview-accept <workspace> <artifact> --accept [--accepted-by ...] [--accepted-at RFC3339] [--reason ...]`.
  - `ouro provider tool-plan-preview-acceptances <workspace> [artifact]` lists or inspects acceptance audit artifacts.
- TUI:
  - Workspace-bound Model Runtime treats valid persisted `model tool plan preview:` artifact rows as double-confirm `x` metadata actions.
  - Invalid/read-only preview rows remain read-only.
  - `model tool plan preview acceptance artifact:` rows render as read-only Inspector targets.

## Boundary

- The accept gate writes accept/select metadata only.
- It does not execute tools, create execution dispatches, dispatch workers, contact endpoints or targets, call providers, run another model/tool round, or persist prompts/full assistant text/raw provider bodies/secrets/executable model instructions.
- Acceptance artifacts are audit history, not execution authorization.
- Selecting the linked compile-preview review still only marks current review identity; review execution remains behind the existing execution/dispatch gates.

## Validation

Focused validation before documentation synchronization passed with:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Test(NewTUIModelForWorkspace(AcceptsModelToolPlanPreviewAfterDoubleConfirm|ShowsModelToolPlanPreviewArtifacts)|AcceptModelToolPlanPreviewArtifact)' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'ToolPlanPreview|ToolPlanPreviews|ProviderToolPlan|RootCommand' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'ModelRuntimeToolPlanPreview|RuntimeTabs|DoesNotRequestReadOnlyRuntimeActions' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'ModelToolPlanPreview|ToolPlanPreview|NewTUIModelForWorkspace(AcceptsModelToolPlanPreview|ShowsModelToolPlanPreview)' -count=1 -v`

Fresh validation after documentation synchronization passed with:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'ToolPlanPreview|ToolPlanPreviews|ProviderToolPlan|ModelRuntime|RuntimeTabs|DoesNotRequestReadOnlyRuntimeActions|RootCommand' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `git diff --check`
- `gofmt -l cmd/ouro/main.go cmd/ouro/main_test.go internal/app/model_tool_plan_preview_artifact.go internal/app/model_tool_plan_preview_artifact_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/app.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`
