# Milestone 211 - AI Chat Local Artifact Counts

## Status

Verified.

## Summary

Workspace-bound AI dialog now handles `/artifacts`, `/artifact-counts`, and `/workflow-artifacts` locally. The command reads the existing read-only workflow summary and returns a compact Mission Conversation response:

```text
workflow artifacts: status=ready stage=model-tool-plan previews=0 acceptances=0 compile_dispatches=0 compile_executions=0 report_dispatches=0 report_executions=0 safety_errors=0 stored=false
```

The local response is metadata-only, not persisted, and excluded from later provider history.

## Safety Boundary

The command does not require provider setup, call provider generation, write provider usage records, write replay events, create model-generation artifacts, create model tool-plan preview artifacts, create compile-preview reviews, create execution dispatches, dispatch workers, execute tools, leak API key values, or contact targets.

The response is count-only. It does not include workspace paths, artifact paths, command strings, provider URLs, prompt text, assistant text, raw provider bodies, bearer tokens, or API key values.

## Validation

Focused RED/GREEN:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogArtifactsCommandDoesNotRequireProvider -count=1 -v
```

Broader regression, build, strict doctor, and hygiene checks passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(ArtifactsCommandDoesNotRequireProvider|CommandCommandDoesNotRequireProvider|NextCommandDoesNotRequireProvider|StatusCommandDoesNotRequireProvider|ContextCommandDoesNotRequireProvider|UsageCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory)' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|ChatCommand|WorkflowStatus|SummarizeWorkspaceWorkflow|ProviderGenerate|GenerateProviderMessage' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
make build
make doctor-strict
gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
git diff --check
rg -n "[ \t]+$" README.md docs/development.md docs/progress/milestone-211-ai-chat-local-artifact-counts.md findings.md progress.md task_plan.md internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
```

## Remaining Completion Boundary

This milestone improves direct chat usability, but it is not a 100% completion claim. Final completion still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
