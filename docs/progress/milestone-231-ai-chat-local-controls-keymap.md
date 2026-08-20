# Milestone 231: AI Chat Local Controls Keymap

## Summary

Workspace-bound AI chat now handles `/keys`, `/shortcuts`, and `/controls` locally. The response gives operators a compact TUI key map without requiring provider setup:

- `chat_focus=i`
- `submit=enter`
- `cancel_draft=esc`
- `quit=q|ctrl+c`
- `runtime_select=up|down`
- `runtime_details=d`
- `runtime_execute=x_then_x`
- `runtime_request=r`
- `runtime_control=R_restart|L_renew|C_cancel|E_expire`
- `replay_step=left|right`
- `replay_jump=home|end|g|G`
- `run_control=c_cancel|p_pause`

`/help` now lists `/keys`, `/shortcuts`, and `/controls` with the rest of the local chat commands.

## Boundaries

The controls keymap path is local and metadata-only. It does not call providers, write replay events, write usage records, create model-generation or model-tool-plan artifacts, create compile-preview reviews, create execution dispatches, dispatch workers, execute tools, or contact targets.

The prompt and response are marked history-excluded, so later provider-backed prompts do not send `/keys`, keymap text, or key names as local-command transcript content to the model.

## Validation

RED first showed `/keys` falling through to the offline provider path and rendering an `AI > error:` row with setup guidance.

Focused and adjacent validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogKeysCommandDoesNotRequireProvider -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(KeysCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory)' -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(KeysCommandDoesNotRequireProvider|QuickstartCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SetupCommandDoesNotRequireProvider|StatusCommandDoesNotRequireProvider|OverviewCommandDoesNotRequireProvider|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory)' -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'AIDialog|ChatCommand|MissionConversation|Workbench|RootCommand' -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
```

The full usable IDE target remains just short of 100% until real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance are complete.
