# Milestone 205 - AI Chat Local Command History Isolation

## Summary

Ouroboros AI chat now excludes local command prompts and local command responses from provider chat history. This prevents local-only UI text such as `/setup`, provider setup guidance, workspace paths, `<ENV_VAR>` placeholders, and configure-command hints from being sent to the model when the operator later submits a normal provider-backed prompt.

Normal non-local user/assistant follow-up history remains enabled, preserving the Codex/Claude Code-like conversation flow without leaking local command metadata.

## Implementation

- Added a `HistoryExcluded` flag to AI dialog results and rendered AI dialog messages.
- Marked local command assistant responses as history-excluded.
- When a local command result is applied, marked the matching user command message as history-excluded too.
- Updated provider history generation to skip history-excluded messages.
- Marked `/status`, `/workflow`, `/workflow-status`, `/help`, and `/setup` local command paths as history-excluded.
- Updated README, development notes, task plan, findings, and this milestone record.

## Safety Boundary

- Local commands still do not call providers.
- Local command text remains visible in the current TUI session but is not sent as provider context later.
- `/setup` output can include workspace-local configure commands for the operator, but those paths and placeholders are excluded from provider history.
- The change does not disable normal non-local follow-up history.
- It does not write replay events, provider usage records, model-generation artifacts, model tool-plan preview artifacts, compile-preview reviews, execution dispatches, worker artifacts, tool execution, or target contact.

## Validation

- RED first confirmed a `/setup` command on a fresh offline workspace was included in a later provider request, including the local workspace path and configure command.
- Focused GREEN: `go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogLocalCommandsDoNotEnterProviderHistory -count=1 -v`.
- Focused history regression confirmed normal non-local user/assistant history still reaches the provider.
- Adjacent AI dialog, provider, chat, and workflow-status regression passed.
- Package regression passed with `go test ./internal/app ./internal/tui ./cmd/ouro -count=1`.
- Documentation command-block tests passed.
- Full repository validation passed with `go test ./... -count=1`.
- `make build` printed `built bin/ouro`.
- `make doctor-strict` printed `workspace: ready`.

Completion remains about 99.99%, not 100%, until real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance are complete.
