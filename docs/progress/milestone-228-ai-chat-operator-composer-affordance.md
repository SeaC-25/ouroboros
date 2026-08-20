# Milestone 228: AI Chat Operator Composer Affordance

## Summary

The Mission Conversation AI dialog now renders the active operator draft as:

```text
You > <draft>|
```

Assistant output remains under `AI > ...`, and submitted user messages remain under `You > ...`.

This makes `ouro chat <workspace>` and the `i`-activated interactive composer read like a real terminal conversation instead of making the editable draft look like an assistant response.

## Safety Boundary

- No provider setup behavior changes.
- No provider call behavior changes.
- No replay database or replay event behavior changes.
- No provider usage, model-generation, model tool-plan preview, compile-preview review, execution dispatch, worker, or report artifact behavior changes.
- No tool is executed, no worker is dispatched, and no target is contacted.
- Prompt text, full assistant text, and in-session history persistence boundaries are unchanged.

## Validation

- RED: `go test ./internal/tui -run 'TestMissionConversationAIDialogComposerRendersAsOperatorInput|TestModelAIDialogInputSubmitsPrompt' -count=1 -v` failed because active drafts rendered as `AI > plan next step`.
- GREEN: the same focused test command passed after rendering active drafts as `You > plan next step|`.
- Adjacent TUI/cmd chat regression passed with `go test ./internal/tui ./cmd/ouro -run 'AIDialog|ChatCommand|MissionConversation|Workbench|RootCommand' -count=1 -v`.
- Touched package regression passed with `go test ./internal/tui ./cmd/ouro -count=1`.
- Documentation bash-block syntax passed with `go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`.
- `make build` printed `built bin/ouro`.
- `./bin/ouro chat --help` printed `Launch the workspace AI chat IDE`.
- `gofmt -l`, `git diff --check`, and touched-file trailing-whitespace scan returned clean.

## Remaining Completion Gap

This improves direct IDE usability, but it is not a 100% completion claim. Final completion still requires operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
