# Milestone 210 - AI Chat Local Command Template

## Status

Verified.

## Summary

Workspace-bound AI dialog now handles `/command`, `/next-command`, and `/workflow-command` locally. The command reads the existing read-only workflow summary and returns a compact Mission Conversation response:

```text
next command: status=ready stage=model-tool-plan next_gate=run-provider-tool-plan-preview reason=no-current-model-tool-plan command=ouro run <workspace> --provider-generate --provider-generate-tool-plan-preview --provider-generate-prompt <prompt> stored=false
```

The local response is metadata-only, not persisted, and excluded from later provider history.

## Safety Boundary

The command does not require provider setup, call provider generation, write provider usage records, write replay events, create model-generation artifacts, create model tool-plan preview artifacts, create compile-preview reviews, create execution dispatches, dispatch workers, execute tools, leak API key values, or contact targets.

The returned command is a template. The workspace root is replaced with `<workspace>`, whitespace is normalized, and prompt placeholders such as `<prompt>` remain explicit. The response does not include the real workspace root, provider URLs, prompt text, assistant text, raw provider bodies, bearer tokens, or API key values.

## Validation

Focused RED/GREEN:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogCommandCommandDoesNotRequireProvider -count=1 -v
```

Broader regression, build, strict doctor, and hygiene checks passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(CommandCommandDoesNotRequireProvider|NextCommandDoesNotRequireProvider|StatusCommandDoesNotRequireProvider|ContextCommandDoesNotRequireProvider|UsageCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory)' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|ChatCommand|WorkflowStatus|SummarizeWorkspaceWorkflow|ProviderGenerate|GenerateProviderMessage' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
make build
make doctor-strict
gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
git diff --check
rg -n "[ \t]+$" README.md docs/development.md docs/progress/milestone-210-ai-chat-local-command-template.md findings.md progress.md task_plan.md internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
```

## Remaining Completion Boundary

This milestone improves direct chat usability, but it is not a 100% completion claim. Final completion still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
