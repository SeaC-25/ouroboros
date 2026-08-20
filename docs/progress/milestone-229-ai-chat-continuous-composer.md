# Milestone 229: AI Chat Continuous Composer

## Summary

The Mission Conversation AI dialog now reopens an empty operator composer after each AI dialog result:

```text
You > |
```

This applies after provider replies, local command responses, provider errors, runtime-row preview results, and `/clear` confirmations. Operators can type a follow-up prompt immediately instead of pressing `i` again after every response.

## Safety Boundary

- No provider setup behavior changes.
- No provider call behavior changes.
- No provider-history filtering changes except test helpers now model continuous composer state.
- No prompt text, full assistant text, or multi-turn history persistence behavior changes.
- No replay database, replay event, report, provider usage, model-generation, model tool-plan preview, compile-preview review, execution dispatch, worker, or report artifact behavior changes.
- No preview is accepted, no workflow gate is selected, no tool is executed, no worker is dispatched, and no target is contacted.
- `Esc` still exits the active composer.

## Validation

- RED: `go test ./internal/tui -run 'TestModelAIDialog(InputSubmitsPrompt|ComposerReopensAfterResult)' -count=1 -v` failed because the UI returned to `You > press i to chat` after an AI result.
- GREEN: the same focused command passed after reopening `You > |` in `WithAIDialogResult`.
- Compatibility repair: existing multi-turn app tests initially failed because they still pressed `i` before every prompt and sent leading `i` characters to the provider; test helpers now open the composer only when needed.
- Focused repaired app regression passed with `go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(SendsCurrentSessionHistory|LocalCommandsDoNotEnterProviderHistory|ClearCommandClearsSessionHistory)' -count=1 -v`.
- Adjacent TUI/app/cmd AI/chat regression passed with `go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|ChatCommand|MissionConversation|Workbench|RootCommand' -count=1 -v`.
- Touched package regression passed with `go test ./internal/tui ./internal/app ./cmd/ouro -count=1`.
- Documentation bash-block syntax passed with `go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- `make build` printed `built bin/ouro`.
- `./bin/ouro chat --help` printed `Launch the workspace AI chat IDE`.
- `gofmt -l`, `git diff --check`, and touched-file trailing-whitespace scan returned clean.

## Remaining Completion Gap

This improves direct chat continuity, but it is not a 100% completion claim. Final completion still requires operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
