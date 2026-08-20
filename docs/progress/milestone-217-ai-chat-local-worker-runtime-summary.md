# Milestone 217 - AI Chat Local Worker Runtime Summary

## Status
Verified.

## Summary
The workspace-bound AI dialog now handles `/workers`, `/agents`, and `/runtime` locally before provider generation.

The command returns a metadata-only runtime summary:

```text
workers: registered=<n> available=<n> busy=<n> draining=<n> offline=<n> stale=<n> local=<n> remote=<n> queue_claims=<n> queue_claimed=<n> queue_dispatched=<n> queue_completed=<n> queue_failed=<n> dispatch_executions=<n> dispatch_dispatched=<n> dispatch_completed=<n> dispatch_failed=<n> transport_requests=<n> transport_pending=<n> transport_ack=<n> transport_completed=<n> transport_failed=<n> dispatch_results=<n> dispatch_results_completed=<n> dispatch_results_failed=<n> leases=<n> active_leases=<n> stale_leases=<n> completed_leases=<n> failed_leases=<n> reclaimed_leases=<n> stored=false
```

It reads only existing worker registry, queue claim, dispatch execution, transport request, dispatch result, and worker lease metadata. It does not call providers, write replay events, create usage/model/tool artifacts, create compile-preview reviews, create execution dispatches, dispatch workers, run tools, or contact targets.

## Safety Boundary
- Does not expose workspace paths, worker IDs, endpoints, labels, run IDs, execution IDs, attempt paths, provider setup commands, API key values, authorization headers, or bearer tokens.
- Marks the local command response as metadata-only, not prompt/response stored, and excluded from provider history.
- Updates `/help` so the command is discoverable from a fresh offline workspace.

## Verification
RED first showed `/workers` still reached the offline provider setup path and leaked the workspace path plus configure command instead of returning a local worker summary.

Focused RED/GREEN and local history isolation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogWorkersCommandDoesNotRequireProvider -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(WorkersCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory)' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(WorkersCommandDoesNotRequireProvider|ProvidersCommandDoesNotRequireProvider|SafetyCommandDoesNotRequireProvider|ToolchainCommandDoesNotRequireProvider|ReportsCommandDoesNotRequireProvider|RunsCommandDoesNotRequireProvider|ArtifactsCommandDoesNotRequireProvider|CommandCommandDoesNotRequireProvider|NextCommandDoesNotRequireProvider|StatusCommandDoesNotRequireProvider|ContextCommandDoesNotRequireProvider|UsageCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory)' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|ChatCommand|WorkflowStatus|SummarizeWorkspaceWorkflow|ProviderGenerate|GenerateProviderMessage|ProviderConfigure|ProviderConfig|ReportProjection|ReplayWorkspace|OpenSQLiteReadOnly|ToolchainContract|ContractCheck|SafetyAuthorization|SafetyBypass|Doctor|RuntimeWorker|WorkerQueue|WorkerLease' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
```

Full repository validation passed with root runtime about 412.305s, `internal/app` about 110.773s, and `cmd/ouro` about 29.873s. `make build` printed `built bin/ouro`; `make doctor-strict` printed `workspace: ready`.
