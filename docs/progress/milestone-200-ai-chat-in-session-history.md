# Milestone 200 - AI Chat In-Session History

## Summary

Ouroboros now carries current-session AI dialog context across follow-up questions. Workspace-bound AI dialog submissions build an ordered provider message list from successful in-memory user prompts and assistant replies, then append the new user prompt before calling the configured `openai_compatible` provider.

This moves `ouro chat <workspace>` closer to a Codex/Claude Code-like security IDE: operators can ask follow-up questions without manually restating the prior answer.

## Implementation

- Added `AIDialogRequest.History` and `AIDialogHistoryMessage` in the TUI model.
- Built history from existing Mission Conversation AI dialog rows before appending the new prompt.
- Included only non-empty `user` rows and successful non-generating `assistant` rows.
- Added `ProviderGenerationMessage` and `ProviderGenerationRequest.Messages` for structured chat payloads.
- Kept legacy `UserPrompt` behavior for CLI/manual provider generation callers.
- Mapped TUI history plus the current prompt into OpenAI-compatible `messages`.

## Safety Boundary

- History is current-session memory only.
- Prompt text, full assistant text, and history are not persisted to replay events, reports, provider usage records, or model-generation review artifacts.
- Failed/generating assistant rows and setup guidance are not sent as assistant history.
- The dialog still does not execute model output, dispatch workers, create execution dispatches, contact targets, or print/store API key values.

## Validation

- RED first confirmed the second AI dialog provider request only sent system plus the new user prompt.
- Focused GREEN: `go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogSendsCurrentSessionHistory -count=1 -v`.
- Adjacent AI/provider/chat regression: `go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|ProviderGenerate|GenerateProviderMessage|ChatCommand|ConfigureOpenAICompatible' -count=1 -v`.
- Package regression: `go test ./internal/app ./internal/tui ./cmd/ouro -count=1`.
- Documentation command-block tests passed.
- Full repository validation passed with `go test ./... -count=1`.
- `make build` printed `built bin/ouro`.
- `make doctor-strict` printed `workspace: ready`.

Completion remains about 99.99%, not 100%, until real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance are complete.
