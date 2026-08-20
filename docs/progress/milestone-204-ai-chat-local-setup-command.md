# Milestone 204 - AI Chat Local Setup Command

## Summary

Ouroboros AI chat now supports a local `/setup` command inside the Mission Conversation dialog. Operators can ask the chat surface for current provider setup state and the `provider configure-openai-compatible` command without first submitting a normal prompt and triggering an offline-provider error.

This improves the direct `ouro chat <workspace>` first-run path for security operators who expect the IDE assistant to explain how to connect a model from inside the conversation.

## Implementation

- Added `/setup` to the AI dialog local-command branch before provider generation.
- Reused the existing `aiProviderSetupGuidanceForWorkspace` and `aiProviderSetupConversationMessage` projection so initial rows, provider errors, and local setup command text stay aligned.
- Updated `/help` to list `/setup`.
- Updated README, development notes, task plan, findings, and this milestone record.

## Safety Boundary

- `/setup` does not call providers.
- It does not append provider setup errors as failures.
- It does not create replay events, provider usage records, model-generation artifacts, model tool-plan preview artifacts, compile-preview reviews, execution dispatches, worker artifacts, tool execution, or target contact.
- It reports `api_key_value_stored=false` and does not print API key values.
- The response is metadata-only and rendered only in the current TUI session with `stored=false`.

## Validation

- RED first confirmed `/help` did not list `/setup` and `/setup` still reached provider generation on a fresh offline workspace.
- Focused GREEN: `go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(HelpCommandDoesNotRequireProvider|SetupCommandDoesNotRequireProvider)' -count=1 -v`.
- Adjacent AI dialog, provider, chat, and workflow-status regression passed.
- Package regression passed with `go test ./internal/app ./internal/tui ./cmd/ouro -count=1`.
- Documentation command-block tests passed.
- Full repository validation passed with `go test ./... -count=1`.
- `make build` printed `built bin/ouro`.
- `make doctor-strict` printed `workspace: ready`.

Completion remains about 99.99%, not 100%, until real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance are complete.
