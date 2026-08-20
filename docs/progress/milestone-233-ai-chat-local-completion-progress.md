# Milestone 233: AI Chat Local Completion Progress

## Summary

Workspace-bound AI chat now handles `/completion`, `/progress`, `/percent`, `/acceptance`, and `/completion-status` locally. The response gives operators a provider-free answer to "how close are we?" without claiming final completion:

- `local_ide_usable=true`
- `direct_chat=true`
- `completion_estimate=99.99`
- `claim_100_ready=false`
- `final_review_status=external_evidence_required`
- production acceptance, completion audit, attestation, manifest review, and operator acceptance requirements

This keeps the direct chat surface honest: the IDE is locally usable, but the final 100% claim still depends on real operator-owned production evidence and final human/operator review.

## Boundaries

The completion/progress path is local and metadata-only. It does not inspect or print workspace paths, setup commands, evidence output filenames, provider URLs, environment-variable names, API key values, provider request bodies, prompt text, or assistant text.

It does not require provider setup, call providers, write replay events, write usage records, create model-generation or model-tool-plan artifacts, create compile-preview reviews, create execution dispatches, dispatch workers, execute tools, or contact targets.

The prompt and response are marked history-excluded, so later provider-backed prompts do not send `/completion`, completion posture text, completion estimates, or final-evidence gate labels as transcript content to the model.

`/help` now lists the completion/progress aliases with the rest of the local chat commands.

## Validation

RED first showed `/completion` falling through to the offline provider path and rendering an `AI > error:` row with setup guidance.

Focused validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogCompletionCommandDoesNotRequireProvider -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(CompletionCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory)' -count=1 -v
```

Focused and adjacent validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogCompletionCommandDoesNotRequireProvider -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(CompletionCommandDoesNotRequireProvider|DoctorCommandDoesNotRequireProvider|KeysCommandDoesNotRequireProvider|QuickstartCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SetupCommandDoesNotRequireProvider|StatusCommandDoesNotRequireProvider|OverviewCommandDoesNotRequireProvider|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory)' -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'AIDialog|ChatCommand|MissionConversation|Workbench|RootCommand|Doctor|WorkflowStatus|ToolchainContract|SafetyAuthorization' -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build

./bin/ouro chat --help

git diff --check

rg -n '[ \t]+$' README.md docs/development.md task_plan.md findings.md progress.md docs/progress/milestone-233-ai-chat-local-completion-progress.md internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go

gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
```

The full usable IDE target remains just short of 100% until real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance are complete.
