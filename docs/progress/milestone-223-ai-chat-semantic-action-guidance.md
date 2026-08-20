# Milestone 223: AI Chat Semantic Action Guidance

## Summary

The workspace-bound AI dialog now gives provider-backed prompts the safe action language needed to suggest an operator-reviewed tool-plan preview. The system prompt and `/context` output include only declared compiler semantic-action IDs, their count, the canonical `semantic_action:<action>` marker format, preview-only mode, and the operator-review requirement.

This makes the chat surface closer to a security-operator Codex/Claude Code: a model can recommend a bounded Ouroboros action in the vocabulary the IDE already understands, while execution stays behind the existing metadata-only preview, acceptance, dispatch, and execute gates.

## Safety Boundary

- No tool names, command arrays, manifest paths, install paths, payload paths, workspace paths, target values, provider URLs, model names, API-key environment names, prompt text, full assistant text, local command transcript text, API key values, `Authorization`, or `Bearer` values are added to provider context.
- The semantic action context is read-only and generated from `compiler.DeclaredSemanticActions()`.
- The canonical provider guidance is `semantic_action:<action>`; legacy `ouro_action:<action>` remains accepted by the existing preview extractor but is not the new prompt guidance format.
- The dialog still does not execute tools, dispatch workers, contact targets, write replay events, write reports, or persist prompt/full assistant text from normal chat.
- A valid marker still creates only the existing metadata-only model tool-plan preview plus linked dry-run compile-preview review; operators must accept and execute through separate gates.

## Validation

- RED: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(SendsReadOnlyWorkspaceContext|ContextCommandDoesNotRequireProvider)' -count=1 -v` failed because `semantic_actions.status=available` was absent from the provider system prompt and `/context`.
- GREEN: the same focused command passed after adding `tuiAIDialogSemanticActionContext()` and moving semantic-action context before overview fields for visible `/context` output.
- Focused AI dialog regression passed with `go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(OverviewCommandDoesNotRequireProvider|ContextCommandDoesNotRequireProvider|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory|LocalCommandsDoNotEnterProviderHistory|HelpCommandDoesNotRequireProvider|CreatesModelToolPlanPreview)' -count=1 -v`.
- Adjacent TUI/app/cmd regression passed with `go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|SemanticAction|ModelToolPlanPreview|Overview|Context|Capabilities|Mission|Evidence|ReportProjection|ReplayWorkspace|OpenSQLiteReadOnly|WorkflowStatus|ProviderGenerate|GenerateProviderMessage|ToolchainContract|SafetyAuthorization|RuntimeWorker' -count=1 -v`.
- Touched package regression passed with `go test ./internal/app ./internal/tui ./cmd/ouro -count=1`.
- Documentation bash-block syntax passed with `go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Full repository validation passed with `go test ./... -count=1`, including root runtime about 412.247s, `internal/app` about 110.602s, and `cmd/ouro` about 28.691s.
- `make build` printed `built bin/ouro`.
- `make doctor-strict` printed `workspace: ready`.
- `gofmt -l`, `git diff --check`, and touched-file trailing-whitespace scan returned clean.

## Remaining Completion Gap

This moves the IDE closer to a usable security-operator assistant, but it is not a 100% completion claim. Final completion still requires operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
