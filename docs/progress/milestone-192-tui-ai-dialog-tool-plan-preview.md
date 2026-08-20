# Milestone 192 - TUI AI Dialog Tool Plan Preview

## Summary

Bridged the interactive TUI AI dialog into the existing metadata-only model tool-plan preview workflow. The Mission Conversation dialog can now turn a valid declared `semantic_action:` or `ouro_action:` marker in the assistant response into a reviewable Model Runtime action without executing tools.

## Behavior

- `run --interactive` and `replay --interactive` still use `i` to open the AI dialog composer.
- Submitted prompts still call the configured `openai_compatible` provider with the Ouroboros security-IDE system prompt.
- If the assistant response contains a valid declared action marker, such as `semantic_action: enumerate_paths`, the workspace-bound submitter creates:
  - one dry-run metadata-only compile-preview review;
  - one `workspace/runtime/model-tool-plan-previews/` artifact with `source=tui-ai-dialog`;
  - current-session Model Runtime rows for the existing `model-tool-plan-preview` TUI action.
- The existing double-confirm `x` acceptance path and `ouro provider tool-plan-preview-accept ... --accept` remain the only accept/select gates.
- Missing, malformed, or unsupported markers do not create preview artifacts.

## Safety Boundary

The dialog-created preview path does not execute tools, create execution dispatches, dispatch workers, contact targets, call another provider round, append replay events, write model-generation artifacts, or write provider usage records. It does not persist prompt text, full assistant text, raw provider bodies, bearer tokens, or executable model instructions. Persisted preview metadata is limited to source/agent/role, provider/model identity, declared action, marker preview, model-output digest, linked review path/counts, artifact digest, and explicit zero side-effect flags.

## Validation

Focused RED/GREEN and adjacent regression passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogCreatesModelToolPlanPreview -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app -run 'AIDialog|ModelToolPlanPreview|NewTUIModelForWorkspaceAIDialog' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(CreatesModelToolPlanPreview|UsesConfiguredProvider)|TestNewTUIModelForWorkspaceAcceptsModelToolPlanPreviewAfterDoubleConfirm|TestNewTUIModelForWorkspaceShowsModelToolPlanPreviewArtifacts|TestModelToolPlanPreviewProjectionIncludesEncodedReferenceFields' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'AIDialog|ModelToolPlanPreview|RuntimeModelToolPlanPreview' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
gofmt -l internal/tui/app.go internal/tui/conversation.go internal/app/model_tool_plan_preview_artifact.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
git diff --check
```

`make build` printed `built bin/ouro`; `make doctor-strict` printed `workspace: ready`; `gofmt -l` and `git diff --check` returned clean.

## Remaining

This milestone improves the core "security Codex" interaction loop, but it is not the final 100% production-completion claim. Final completion still requires real operator-owned production evidence, an approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
