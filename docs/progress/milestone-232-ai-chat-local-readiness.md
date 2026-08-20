# Milestone 232: AI Chat Local Readiness

## Summary

Workspace-bound AI chat now handles `/doctor`, `/readiness`, and `/health` locally. The response gives operators a provider-free readiness line:

- `workspace_ready`
- `strict_ready`
- warning/error and strict-error counts
- safety-bypass count
- provider setup status and enabled-provider count
- `chat` and `tool_planning` capability booleans
- workflow status and next gate
- toolchain contract status and tool count

This gives a fresh chat session an immediate answer to whether the workspace is usable before configuring a real AI provider.

## Boundaries

The readiness path is local and metadata-only. It reuses existing metadata-only doctor, provider setup, workflow status, and toolchain contract projections. It does not call providers, write replay events, write usage records, create model-generation or model-tool-plan artifacts, create compile-preview reviews, create execution dispatches, dispatch workers, execute tools, or contact targets.

The prompt and response are marked history-excluded, so later provider-backed prompts do not send `/doctor`, readiness text, workspace paths, setup commands, API-key metadata, or local health details as transcript content to the model.

`/help` now lists `/doctor`, `/readiness`, and `/health` with the rest of the local chat commands.

## Validation

RED first showed `/doctor` falling through to the offline provider path and rendering an `AI > error:` row with setup guidance.

Focused validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogDoctorCommandDoesNotRequireProvider -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(DoctorCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory)' -count=1 -v
```

Focused and adjacent validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(DoctorCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory)' -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(DoctorCommandDoesNotRequireProvider|KeysCommandDoesNotRequireProvider|QuickstartCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SetupCommandDoesNotRequireProvider|StatusCommandDoesNotRequireProvider|OverviewCommandDoesNotRequireProvider|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory)' -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'AIDialog|ChatCommand|MissionConversation|Workbench|RootCommand|Doctor|WorkflowStatus|ToolchainContract|SafetyAuthorization' -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build

./bin/ouro chat --help

git diff --check

rg -n "[ \t]+$" README.md docs/development.md task_plan.md findings.md progress.md docs/progress/milestone-232-ai-chat-local-readiness.md internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go

gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
```

The full usable IDE target remains just short of 100% until real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance are complete.
