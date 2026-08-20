# Milestone 224: AI Chat Local Semantic Actions

## Summary

The workspace-bound AI dialog now has a provider-free `/actions` command family:

- `/actions`
- `/semantic-actions`
- `/action-list`

The command returns declared compiler semantic-action IDs, count, canonical `semantic_action:<action>` marker format, preview-only mode, operator-review requirement, and `stored=false`.

This pairs with M223. The provider prompt already receives the safe action language; operators can now inspect that same action language locally before asking the model to propose a tool-plan preview.

## Safety Boundary

- No provider call is made.
- No replay database or replay event is created.
- No provider usage, model-generation, model tool-plan preview, compile-preview review, execution dispatch, worker, or report artifact is written.
- No tool is executed, no worker is dispatched, and no target is contacted.
- The command does not expose tool names, command arrays, manifest paths, install paths, payload paths, workspace paths, target values, provider URLs, model names, API-key environment names, API key values, `Authorization`, or `Bearer` values.
- The prompt and response are excluded from later provider history.

## Validation

- RED: `go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(ActionsCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory)' -count=1 -v` failed because `/actions` fell through to the offline provider path, `/help` did not list it, and `/actions` entered provider history.
- GREEN: the same focused command passed after adding local `/actions`, `/semantic-actions`, and `/action-list` handling plus help/history updates.
- Focused AI dialog regression passed with `go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(ActionsCommandDoesNotRequireProvider|OverviewCommandDoesNotRequireProvider|ContextCommandDoesNotRequireProvider|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory|LocalCommandsDoNotEnterProviderHistory|HelpCommandDoesNotRequireProvider|CreatesModelToolPlanPreview)' -count=1 -v`.
- Adjacent TUI/app/cmd regression passed with `go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|Actions|SemanticAction|ModelToolPlanPreview|Overview|Context|Capabilities|Mission|Evidence|ReportProjection|ReplayWorkspace|OpenSQLiteReadOnly|WorkflowStatus|ProviderGenerate|GenerateProviderMessage|ToolchainContract|SafetyAuthorization|RuntimeWorker' -count=1 -v`.
- Touched package regression passed with `go test ./internal/app ./internal/tui ./cmd/ouro -count=1`.
- Documentation bash-block syntax passed with `go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Full repository validation passed with `go test ./... -count=1`, including root runtime about 414.877s, `internal/app` about 114.424s, and `cmd/ouro` about 29.701s.
- `make build` printed `built bin/ouro`.
- `make doctor-strict` printed `workspace: ready`.
- `gofmt -l`, `git diff --check`, and touched-file trailing-whitespace scan returned clean.

## Remaining Completion Gap

This improves direct IDE usability, but it is not a 100% completion claim. Final completion still requires operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
