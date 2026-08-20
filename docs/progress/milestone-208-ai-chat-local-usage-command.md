# Milestone 208 - AI Chat Local Usage Command

## Status

Verified.

## Summary

Workspace-bound AI dialog now handles `/usage` and `/provider-usage` locally. The command reads the existing metadata-only provider usage ledger and returns a compact Mission Conversation response:

```text
provider usage: records=2 providers=local-openai|second-openai models=gpt-a|gpt-b ... stored=false
```

The local response is metadata-only, not persisted, and excluded from later provider history.

## Safety Boundary

The command does not require provider setup, call provider generation, write new provider usage records, write replay events, create model-generation artifacts, create model tool-plan preview artifacts, create compile-preview reviews, create execution dispatches, dispatch workers, execute tools, leak persisted prompt/assistant text, leak API key values, or contact targets.

The summary is limited to usage metadata: record count, provider/model names, token totals, estimated cost aggregate, budget counters, and first/last record timestamps. It does not include the usage directory path, prompt text, assistant text, raw provider bodies, bearer tokens, provider URLs, or workspace paths.

## Validation

Focused RED/GREEN:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogUsageCommandDoesNotRequireProvider -count=1 -v
```

Broader regression, build, strict doctor, and hygiene checks passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(UsageCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider)' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspace(AIDialog(UsageCommandDoesNotRequireProvider|ContextCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory)|ShowsProviderUsageSummaryReadOnly|ProviderUsageSummaryDoesNotMutateReplayEvents|ProviderUsageSummaryDetailsHandleSpacePathsAndNames)' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|ChatCommand|ProviderUsage|provider usage|UsageCommand|GenerateProviderMessage|ProviderGenerate' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
make build
make doctor-strict
gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
git diff --check
rg -n "[ \t]+$" README.md docs/development.md docs/progress/milestone-208-ai-chat-local-usage-command.md findings.md progress.md task_plan.md internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
```

## Remaining Completion Boundary

This milestone improves direct chat usability, but it is not a 100% completion claim. Final completion still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
