# Milestone 206 - AI Chat Local Clear Command

## Summary

Ouroboros AI chat now supports a local `/clear` command inside the Mission Conversation dialog. Operators can reset the current in-session AI transcript and provider history without deleting persisted workspace evidence or triggering provider generation.

This brings the chat surface closer to Codex/Claude Code-like interaction: old context can be dropped before starting a fresh question, while workspace artifacts and audit state remain untouched.

## Implementation

- Added a `ClearHistory` flag to AI dialog results.
- When a clear result is applied successfully, the TUI replaces the current AI dialog message list with a single local confirmation.
- Added `/clear` to the AI dialog local-command branch before provider generation.
- Marked the `/clear` confirmation as history-excluded.
- Updated `/help` to list `/clear`.
- Updated README, development notes, task plan, findings, and this milestone record.

## Safety Boundary

- `/clear` does not call providers.
- It clears only current in-memory AI dialog messages and provider history.
- It does not delete persisted workspace artifacts, replay events, reports, provider usage records, model-generation artifacts, model tool-plan preview artifacts, compile-preview reviews, execution dispatches, worker artifacts, or tool outputs.
- It does not execute tools, dispatch workers, or contact targets.
- The confirmation is local, metadata-only, history-excluded, and rendered with `stored=false`.

## Validation

- RED first confirmed `/clear` was treated as a normal provider prompt and carried old conversation history.
- Focused GREEN: `go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogClearCommandClearsSessionHistory -count=1 -v`.
- Focused regressions confirmed `/help`, `/setup`, local-command history isolation, and normal non-local provider history still work.
- Adjacent AI dialog, provider, chat, and workflow-status regression passed.
- Package regression passed with `go test ./internal/app ./internal/tui ./cmd/ouro -count=1`.
- Documentation command-block tests passed.
- Full repository validation passed with `go test ./... -count=1`.
- `make build` printed `built bin/ouro`.
- `make doctor-strict` printed `workspace: ready`.

Completion remains about 99.99%, not 100%, until real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance are complete.
