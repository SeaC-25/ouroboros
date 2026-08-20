# Milestone 213 - AI Chat Local Report Status

## Status

Verified.

## Summary

Workspace-bound AI dialog now handles `/reports`, `/report-status`, and `/current-report` locally. The command reads report projection dispatch/execution counts and the current report selection marker as metadata, then returns a compact Mission Conversation response:

```text
reports: dispatches=1 executions=1 current=true run=run-123 events=45 written=true stored=false
```

The local response is metadata-only, not persisted, and excluded from later provider history.

## Safety Boundary

The command does not require provider setup, call provider generation, write provider usage records, write replay events, create model-generation artifacts, create model tool-plan preview artifacts, create compile-preview reviews, create report projection dispatches or executions, write Markdown reports, dispatch workers, execute tools, leak API key values, or contact targets.

The response is summary-only. It does not include workspace paths, report paths, artifact paths, command strings, provider URLs, prompt text, assistant text, raw provider bodies, bearer tokens, or API key values.

## Validation

Focused RED/GREEN:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogReportsCommandDoesNotRequireProvider -count=1 -v
```

Broader regression, build, strict doctor, and hygiene checks passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(ReportsCommandDoesNotRequireProvider|RunsCommandDoesNotRequireProvider|ArtifactsCommandDoesNotRequireProvider|CommandCommandDoesNotRequireProvider|NextCommandDoesNotRequireProvider|StatusCommandDoesNotRequireProvider|ContextCommandDoesNotRequireProvider|UsageCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory)' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|ChatCommand|WorkflowStatus|SummarizeWorkspaceWorkflow|ProviderGenerate|GenerateProviderMessage|ReportProjection|ReplayWorkspace|OpenSQLiteReadOnly' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
make build
make doctor-strict
gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
git diff --check
rg -n "[ \t]+$" README.md docs/development.md docs/progress/milestone-213-ai-chat-local-report-status.md findings.md progress.md task_plan.md internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
```

## Remaining Completion Boundary

This milestone improves direct chat usability, but it is not a 100% completion claim. Final completion still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
