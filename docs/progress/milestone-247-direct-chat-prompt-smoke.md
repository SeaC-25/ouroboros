# Milestone 247: Direct Chat Prompt Smoke Binding

## Summary

Added a scriptable direct-chat submission path for smoke and final-evidence checks:

- `ouro chat <workspace> --prompt "..."` now submits one prompt through the same workspace AI dialog submitter used by the interactive TUI.
- The command prints grep-friendly metadata lines with `stored=false`, including provider/model/token metadata, bounded assistant text, and tool-plan preview status.
- A valid semantic-action marker from the fake provider creates one metadata-only model tool-plan preview artifact.
- The prompt path does not launch the interactive TUI and does not create `replay/events.db`.
- Empty explicit `--prompt` values fail with `chat prompt is required` instead of falling through to an interactive session.

`scripts/smoke_ai_provider.sh` now exercises this path after provider generation, verifies the fake action marker, verifies one preview artifact, checks no replay DB was created, checks the temporary API key value is not printed, and emits:

```text
chat-prompt: preview=created stored=false
```

Production acceptance and completion audit now require that marker from the canonical IDE AI-provider child log before final review evidence can be published.

## Boundary

This is a direct-use and evidence-binding improvement. It does not make model output executable, does not persist prompts or full assistant text, does not contact real providers in smoke, does not contact targets, and does not claim 100%.

True completion still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final operator acceptance.

## Validation

RED:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run TestChatCommandPromptRejectsEmptyPromptWithoutLaunchingInteractiveSession -count=1 -v
```

failed before the CLI checked whether `--prompt` was explicitly set, because an empty prompt fell through to the interactive chat session.

GREEN:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run TestChatCommandPromptRejectsEmptyPromptWithoutLaunchingInteractiveSession -count=1 -v
```

passed after `chat` routed all explicit `--prompt` invocations through `SubmitWorkspaceAIDialogPrompt`.

Focused chat regression:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'TestChatCommand(PromptSubmitsThroughAIDialogAndCreatesPreview|PromptRejectsEmptyPromptWithoutLaunchingInteractiveSession|LaunchesAIDialogComposerWithoutRunningWorkspace|TUISnapshotPrintsFocusedComposer)' -count=1 -v
```

passed.

Script and final-evidence regression:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(AIProviderSmokeScriptExercisesConfigureAndGeneratePath|CompletionAuditScriptRejectsStageLogWithoutOKSentinel|DocumentationBashBlocksPassShellSyntaxCheck|FinalEvidenceRunbookBashBlocksPassShellSyntaxCheck|FinalEvidenceRunbookDocumentsRequiredFields|ReleaseMakefileAndCIEntrypoints)' -count=1 -v
sh -n scripts/smoke_ai_provider.sh scripts/smoke_production_acceptance.sh scripts/audit_completion.sh
gofmt -l cmd/ouro/main.go cmd/ouro/main_test.go internal/app/tui_snapshot.go release_scripts_test.go
```

passed.

Built-binary and smoke validation:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-ai-provider
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-ide
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -count=1
git diff --check
```

passed. `make smoke-ai-provider` printed `chat-prompt: preview=created stored=false` and `ai-provider-smoke: ok`; `make smoke-ide` printed `direct-use: ok`, `ai-provider: ok`, and `ide-smoke: ok`.

## Sidecar Review

A read-only subagent review was attempted for the final-evidence marker binding, but the environment returned `429 Too Many Requests` after retries. A second sidecar spawn hit `agent thread limit reached`. Local grep, RED/GREEN tests, script syntax, build, and smoke output are the authoritative evidence for this milestone.
