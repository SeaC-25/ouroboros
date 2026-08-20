# Milestone 234: AI Chat Local Final Evidence Runbook

## Summary

Workspace-bound AI chat now handles `/final`, `/final-evidence`, `/final-review`, `/audit`, `/audit-completion`, and `/production-acceptance` as local metadata-only commands.

The response summarizes the remaining final evidence path:

- production acceptance through `make_smoke-production-acceptance`
- operator payload inventory placeholder
- deployment policy placeholder
- completion attestation placeholder
- completion audit through `make_audit-completion`
- manifest and digest-sidecar review
- operator acceptance before any 100% claim

## Boundaries

The command does not require provider setup, call providers, write replay/usage/model/tool/worker artifacts, execute tools, dispatch workers, contact targets, expose workspace paths, expose provider setup commands, expose output filenames, expose API keys, or enter later provider history.

This is runbook guidance only. It does not perform production acceptance, completion audit, attestation, final review, or a 100% completion claim.

## Validation

Focused GREEN passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(FinalEvidenceCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory)' -count=1 -v
```

Final verification also passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogFinalEvidenceCommandDoesNotRequireProvider -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(FinalEvidenceCommandDoesNotRequireProvider|CompletionCommandDoesNotRequireProvider|DoctorCommandDoesNotRequireProvider|KeysCommandDoesNotRequireProvider|QuickstartCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SetupCommandDoesNotRequireProvider|StatusCommandDoesNotRequireProvider|OverviewCommandDoesNotRequireProvider|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory)' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'AIDialog|ChatCommand|MissionConversation|Workbench|RootCommand|Doctor|WorkflowStatus|ToolchainContract|SafetyAuthorization' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
./bin/ouro chat --help
git diff --check
rg -n '[ \t]+$' README.md docs/development.md task_plan.md findings.md progress.md docs/progress/milestone-234-ai-chat-local-final-evidence-runbook.md internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
```
