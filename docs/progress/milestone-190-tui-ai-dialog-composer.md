# Milestone 190 - TUI AI Dialog Composer

## Summary

Added a direct AI dialog composer to the workspace-bound Bubble Tea TUI so `ouro run --interactive` and `ouro replay --interactive` are no longer only replay/workflow row browsers. The Mission Conversation panel now supports an `i`-activated prompt draft, `Enter` submission, `Esc` cancellation, and current-session user/AI transcript rendering.

## Behavior

- Pressing `i` focuses the Mission Conversation AI composer.
- Typed runes and spaces update the visible `AI >` draft line.
- `Enter` submits a non-empty prompt through a TUI-injected submitter.
- `Esc` cancels the draft without submitting.
- Successful responses render as `AI > <text>` plus provider/model/token metadata.
- Failed provider calls render a sanitized `AI > error:` row.

Workspace-bound TUI models inject a submitter that calls the configured `openai_compatible` provider through `GenerateProviderMessage`. The request includes an Ouroboros security-IDE system prompt for authorized security work, scoped objectives, evidence review, toolchain state, and operator-reviewed gates.

## Safety Boundary

The interactive dialog is an advisory conversation surface only. It does not persist prompt text or full assistant text to replay events, reports, provider usage records, or model-generation review artifacts. It displays `stored=false` in the TUI metadata row. Model output from this path does not execute commands, run tools, dispatch workers, contact targets, or select workflow gates; existing explicit operator gates remain the only mutation and execution paths.

## Validation

Focused RED/GREEN and adjacent regression passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app -run 'TestModelAIDialogInputSubmitsPrompt|TestNewTUIModelForWorkspaceAIDialogUsesConfiguredProvider' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialogUsesConfiguredProvider|TestNewTUIModelForWorkspaceShowsProviderUsageSummaryReadOnly|TestRunWorkspaceDoesNotGenerateProviderMessageByDefault|TestGenerateProviderMessageCallsOpenAICompatibleChatCompletions' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'TestRunCommandCanPrintTUISnapshot|TestRunCommandInteractiveStreamsSnapshotsToTUI|TestReplayCommandCanPrintTUISnapshot|TestReplayCommandInteractive' -count=1 -v
```

## Remaining

This milestone improves the core IDE interaction, but it is not the final 100% production-completion claim. Final completion still requires real operator-owned production evidence, an approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
