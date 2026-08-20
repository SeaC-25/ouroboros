# Milestone 225: AI Chat Preview Notification Hardening

## Summary

Mission Conversation now keeps AI-dialog-created tool-plan preview notifications path-free.

When a provider response contains a valid declared `semantic_action:` or `ouro_action:` marker, the workspace-bound AI dialog still creates the metadata-only model tool-plan preview artifact, creates the linked dry-run compile-preview review, appends Model Runtime rows, and selects the created preview row. The Mission Conversation transcript now renders only:

- preview status
- semantic action
- marker preview
- `artifact_created=true|false`
- `stored=false`

The artifact path remains available in Model Runtime rows and Inspector details, where the existing preview acceptance gates already operate.

## Safety Boundary

- Prompt text and full assistant text are still not persisted by the interactive dialog path.
- Mission Conversation no longer renders the model tool-plan preview artifact path.
- Model Runtime and Inspector remain the path-level review surfaces for preview artifacts.
- The change does not create execution dispatches, execute tools, dispatch workers, call providers beyond the submitted chat prompt, or contact targets.
- The existing double-confirm `x` and `tool-plan-preview-accept --accept` gates remain required before a preview can select the linked compile-preview review.

## Validation

- RED: `go test ./internal/tui -run TestMissionConversationAIDialogPreviewOmitsArtifactPath -count=1 -v` failed because the preview notification rendered `path=/tmp/.../model-tool-plan-previews/preview.json`.
- GREEN: the same focused test passed after rendering `artifact_created=true|false` instead of `path=<artifact>`.
- Focused TUI regression passed with `go test ./internal/tui -run 'MissionConversation|AIDialog|ModelUpdateDoesNotExecuteWorkflowStatusRows|RuntimeModelToolPlanPreview' -count=1 -v`.
- Focused app AI dialog regression passed with `go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialogCreatesModelToolPlanPreview|TestNewTUIModelForWorkspaceAIDialog.*CommandDoesNotRequireProvider|TestNewTUIModelForWorkspaceAIDialogLocalCommandsDoNotEnterProviderHistory|TestNewTUIModelForWorkspaceAIDialogSendsReadOnlyWorkspaceContext|TestNewTUIModelForWorkspaceAIDialogSendsCurrentSessionHistory' -count=1 -v`.
- Adjacent TUI/app/cmd regression passed with `go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|MissionConversation|ModelToolPlanPreview|WorkflowStatus|RuntimeTabs|ProviderGenerate|ToolPlanPreview|Actions|SemanticAction' -count=1 -v`.
- Touched package regression passed with `go test ./internal/app ./internal/tui ./cmd/ouro -count=1`, including `internal/app` about 78.054s, `internal/tui` about 0.124s, and `cmd/ouro` about 23.279s.
- Documentation bash-block syntax passed with `go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Full repository validation passed with `go test ./... -count=1`, including root runtime about 411.389s, `internal/app` about 112.076s, and `cmd/ouro` about 29.402s.
- `make build` printed `built bin/ouro`.
- `make doctor-strict` printed `workspace: ready`.
- `gofmt -l`, `git diff --check`, and touched-file trailing-whitespace scan returned clean.

## Remaining Completion Gap

This improves direct IDE transcript safety, but it is not a 100% completion claim. Final completion still requires operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
