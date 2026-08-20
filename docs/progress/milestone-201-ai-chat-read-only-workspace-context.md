# Milestone 201 - AI Chat Read-Only Workspace Context

## Summary

Ouroboros AI chat now includes compact read-only workflow context in the system prompt for workspace-bound AI dialog submissions. This gives `ouro chat <workspace>` current next-gate awareness without making model output executable or exposing local paths.

## Implementation

- Added workspace-aware system prompt generation for the TUI AI dialog.
- Reused `SummarizeWorkspaceWorkflow` as the read-only source of workflow state.
- Included only compact context fields:
  - workflow status, mode, stage, next gate, reason, and action
  - safety-error count
  - aggregate preview, acceptance, compile dispatch/execution, and report dispatch/execution counts
- Kept current-session chat history as ordered user/assistant messages after the system prompt.

## Safety Boundary

- Context generation is read-only.
- The system prompt omits workspace paths, artifact paths, command strings, provider URLs, prompt text, full assistant text, and API key values.
- The AI dialog still does not write replay events, provider usage records, model-generation artifacts, execution dispatches, worker artifacts, tool execution, or target contact.
- Provider setup errors remain sanitized and still show the setup command when the default offline provider is active.

## Validation

- RED first confirmed the TUI AI dialog provider system prompt lacked `Workspace context:`.
- Focused GREEN: `go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogSendsReadOnlyWorkspaceContext -count=1 -v`.
- Adjacent AI/provider/chat/workflow regression passed.
- Package regression passed with `go test ./internal/app ./internal/tui ./cmd/ouro -count=1`.
- Documentation command-block tests passed.
- Full repository validation passed with `go test ./... -count=1`.
- `make build` printed `built bin/ouro`.
- `make doctor-strict` printed `workspace: ready`.

Completion remains about 99.99%, not 100%, until real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance are complete.
