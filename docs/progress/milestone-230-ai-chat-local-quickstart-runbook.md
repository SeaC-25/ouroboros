# Milestone 230: AI Chat Local Quickstart Runbook

## Summary

Workspace-bound AI chat now handles `/quickstart`, `/use`, and `/runbook` locally. The response gives operators a provider-free first-use runbook:

- `./bin/ouro init <workspace>`
- `./bin/ouro chat <workspace>`
- `ouro provider configure-openai-compatible <workspace> --name local-openai --base-url <base-url> --model <model> --api-key-env <ENV_VAR> --capability chat --capability tool_planning --enable`
- `make smoke-ide`
- a compact pointer to the most useful local chat commands

The command returns `api_key_value_stored=false` and `stored=false`. It uses placeholders for workspace, base URL, model, and API-key environment variable, and the quickstart response itself does not expose the actual workspace path or API-key value.

## Boundaries

The quickstart path is local and metadata-only. It does not require provider setup, call providers, write replay events, write usage records, create model-generation or model-tool-plan artifacts, create compile-preview reviews, create execution dispatches, dispatch workers, execute tools, or contact targets.

The prompt and response are marked history-excluded, so later provider-backed prompts do not send `/quickstart`, quickstart runbook text, command skeletons, `<workspace>`, `<base-url>`, `<ENV_VAR>`, workspace paths, or setup guidance to the model.

`/help` now lists `/quickstart`, `/use`, and `/runbook` with the rest of the local commands.

## Validation

RED first showed `/quickstart` falling through to the offline provider path and rendering an `AI > error:` row with setup guidance.

Focused and adjacent validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogQuickstartCommandDoesNotRequireProvider -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(QuickstartCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory)' -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(QuickstartCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SetupCommandDoesNotRequireProvider|StatusCommandDoesNotRequireProvider|OverviewCommandDoesNotRequireProvider|CapabilitiesCommandDoesNotRequireProvider|MissionCommandDoesNotRequireProvider|EvidenceCommandDoesNotRequireProvider|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory)' -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|ChatCommand|MissionConversation|Workbench|RootCommand' -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1

GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build

./bin/ouro chat --help

git diff --check

rg -n "[ \t]+$" README.md docs/development.md task_plan.md findings.md progress.md internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go

gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
```

The full usable IDE target remains just short of 100% until real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance are complete.
