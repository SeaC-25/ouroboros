# Milestone 227: AI Chat Local Review Gates

## Summary

The workspace-bound AI dialog now has a provider-free review-gates command family:

- `/gates`
- `/review-gates`
- `/gate-status`

The command reports a path-free posture line for the existing workflow gates:

- workflow status, stage, next gate, reason, and semantic action
- model-preview and acceptance counts
- compile dispatch and execution counts
- report dispatch and execution counts
- current-report selected boolean
- safety-error count
- command-availability boolean
- `stored=false`

This gives operators a chat-local answer to "which review gate is open or blocked?" without turning Mission Conversation into a path-level artifact browser.

## Safety Boundary

- No provider setup is required.
- No provider call is made.
- No replay database or replay event is created.
- No provider usage, model-generation, model tool-plan preview, compile-preview review, execution dispatch, worker, or report artifact is written.
- No preview is accepted, no compile-preview selection marker is written, no tool is executed, no worker is dispatched, and no target is contacted.
- The command does not expose artifact paths, workspace paths, command strings, provider URLs, provider names, model names, API-key environment names, API key values, prompt text, full assistant text, `Authorization`, or `Bearer` values.
- The prompt and response are excluded from later provider history.

## Validation

- RED: `go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogReviewGatesCommandDoesNotRequireProvider -count=1 -v` failed because `/gates` fell through to the offline provider path and rendered setup guidance plus workspace paths.
- GREEN: the same focused test passed after adding local `/gates`, `/review-gates`, and `/gate-status` handling.
- Focused local-command regression passed with `go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(ReviewGatesCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory)' -count=1 -v`.
- Focused AI dialog regression passed with `go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(ReviewGatesCommandDoesNotRequireProvider|PreviewsCommandDoesNotRequireProvider|ArtifactsCommandDoesNotRequireProvider|RunsCommandDoesNotRequireProvider|ReportsCommandDoesNotRequireProvider|ToolchainCommandDoesNotRequireProvider|CapabilitiesCommandDoesNotRequireProvider|OverviewCommandDoesNotRequireProvider|ActionsCommandDoesNotRequireProvider|ContextCommandDoesNotRequireProvider|UsageCommandDoesNotRequireProvider|ProvidersCommandDoesNotRequireProvider|WorkersCommandDoesNotRequireProvider|EvidenceCommandDoesNotRequireProvider|MissionCommandDoesNotRequireProvider|SafetyCommandDoesNotRequireProvider|SetupCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory|CreatesModelToolPlanPreview)' -count=1 -v`.
- Adjacent TUI/app/cmd regression passed with `go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|ReviewGates|Gates|Previews|ModelToolPlanPreview|WorkflowStatus|ProviderGenerate|GenerateProviderMessage|Actions|SemanticAction|RuntimeTabs' -count=1 -v`.
- Touched package regression passed with `go test ./internal/app ./internal/tui ./cmd/ouro -count=1`, including `internal/app` about 78.381s, `internal/tui` about 0.114s, and `cmd/ouro` about 22.825s.
- Documentation bash-block syntax passed with `go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- Full repository validation passed with `go test ./... -count=1`, including root runtime about 413.330s, `internal/app` about 112.586s, and `cmd/ouro` about 30.484s.
- `make build` printed `built bin/ouro`.
- `make doctor-strict` printed `workspace: ready`.
- `gofmt -l`, `git diff --check`, and touched-file trailing-whitespace scan returned clean.

## Remaining Completion Gap

This improves direct IDE usability, but it is not a 100% completion claim. Final completion still requires operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
