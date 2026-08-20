# Milestone 215 - AI Chat Local Safety Boundary

## Status

Verified.

## Summary

Workspace-bound AI dialog now handles `/safety`, `/scope`, and `/authorization` locally. The command reads metadata-only normal and strict doctor summaries, then returns a compact Mission Conversation response:

```text
safety: ready=true strict_ready=false warnings=4 errors=0 strict_errors=2 bypasses=2 authorized=0 missing=2 expired=0 stored=false
```

The local response is metadata-only, not persisted, and excluded from later provider history.

## Safety Boundary

The command does not require provider setup, call provider generation, write provider usage records, write replay events, create model-generation artifacts, create model tool-plan preview artifacts, create compile-preview reviews, create execution dispatches, dispatch workers, run tools, probe container runtime, leak API key values, or contact targets.

The response is summary-only. It does not include workspace paths, `scope.yaml`, `authorization.yaml`, safety control names, artifact paths, command strings, provider URLs, prompt text, assistant text, raw provider bodies, bearer tokens, or API key values.

## Validation

Focused RED/GREEN:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogSafetyCommandDoesNotRequireProvider -count=1 -v
```

Focused AI dialog local-command regression:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(SafetyCommandDoesNotRequireProvider|ToolchainCommandDoesNotRequireProvider|ReportsCommandDoesNotRequireProvider|RunsCommandDoesNotRequireProvider|ArtifactsCommandDoesNotRequireProvider|CommandCommandDoesNotRequireProvider|NextCommandDoesNotRequireProvider|StatusCommandDoesNotRequireProvider|ContextCommandDoesNotRequireProvider|UsageCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory)' -count=1 -v
```

Broader regression, build, strict doctor, and hygiene checks passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|ChatCommand|WorkflowStatus|SummarizeWorkspaceWorkflow|ProviderGenerate|GenerateProviderMessage|ReportProjection|ReplayWorkspace|OpenSQLiteReadOnly|ToolchainContract|ContractCheck|SafetyAuthorization|SafetyBypass|Doctor' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
make build
make doctor-strict
gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
git diff --check
rg -n "[ \t]+$" README.md docs/development.md docs/progress/milestone-215-ai-chat-local-safety-boundary.md findings.md progress.md task_plan.md internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
```

## Remaining Completion Boundary

This milestone improves direct chat usability, but it is not a 100% completion claim. Final completion still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
