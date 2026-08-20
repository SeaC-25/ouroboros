# Milestone 203 - AI Chat Local Help Command

## Summary

Ouroboros AI chat now supports a local `/help` command inside the Mission Conversation dialog. Operators can discover the supported local commands and the provider-backed normal prompt boundary from a fresh offline workspace, without configuring or calling an AI provider first.

This improves the direct `ouro chat <workspace>` experience for security operators who expect a Codex/Claude Code-like conversation surface to explain its own interaction model.

## Implementation

- Added `/help` to the AI dialog local-command branch before provider generation.
- Returned a compact in-session response listing `/status`, `/workflow`, `/workflow-status`, and `/help`.
- Kept normal prompts on the existing provider path with workspace context, current-session history, and advisory-only output.
- Updated README, development notes, task plan, findings, and this milestone record.

## Safety Boundary

- `/help` does not call providers.
- It does not append provider setup errors.
- It does not create replay events, provider usage records, model-generation artifacts, model tool-plan preview artifacts, compile-preview reviews, execution dispatches, worker artifacts, tool execution, or target contact.
- The response is metadata-only and rendered only in the current TUI session with `stored=false`.

## Validation

- RED first confirmed `/help` on a fresh offline workspace still tried provider generation and rendered an offline-provider error.
- Focused GREEN: `go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogHelpCommandDoesNotRequireProvider -count=1 -v`.
- Adjacent AI dialog, provider, chat, and workflow-status regression passed.
- Package regression passed with `go test ./internal/app ./internal/tui ./cmd/ouro -count=1`.
- Documentation command-block tests passed.
- Full repository validation passed with `go test ./... -count=1`.
- `make build` printed `built bin/ouro`.
- `make doctor-strict` printed `workspace: ready`.

Completion remains about 99.99%, not 100%, until real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance are complete.
