# Milestone 221: AI Chat Local Overview

## Summary

Added local AI dialog overview commands:

- `/overview`
- `/summary`
- `/dashboard`

The commands run before provider generation and compose existing local read-only summaries for workflow, mission/scope, safety, providers, capabilities, evidence, reports, and workers into one count/status-only posture line:

```text
overview: workflow_status=<status> stage=<stage> next_gate=<gate> mission_configured=<bool> target_policy=<policy> isolation=<mode> targets=<n> safety_ready=<bool> strict_ready=<bool> providers_status=<status> providers=<n> provider_enabled=<n> chat=<bool> tool_planning=<bool> capabilities_status=<status> domain_packs=<n> tools=<n> evidence_runs=<n> findings=<n> reports_current=<bool> workers_registered=<n> worker_queue_claims=<n> stored=false
```

## Safety Boundary

The local overview:

- does not call providers
- does not create `replay/events.db`
- does not write replay, report, model-generation, model-usage, model-tool-plan, compile-preview, worker, or execution artifacts
- does not run health probes, execute tools, dispatch workers, or contact targets
- does not expose mission names, goal text, target values, exclusions, provider URLs, model names, API-key environment names, API key values, workspace paths, command arrays, manifest paths, payload paths, install paths, report paths, `Authorization`, or `Bearer` tokens
- is excluded from later provider history together with other local command prompts/responses

## Validation

Focused RED first failed because `/overview` fell through to provider generation and leaked provider setup guidance. Focused GREEN passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogOverviewCommandDoesNotRequireProvider -count=1 -v
```

Local AI dialog command regression passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(OverviewCommandDoesNotRequireProvider|CapabilitiesCommandDoesNotRequireProvider|MissionCommandDoesNotRequireProvider|EvidenceCommandDoesNotRequireProvider|WorkersCommandDoesNotRequireProvider|ProvidersCommandDoesNotRequireProvider|SafetyCommandDoesNotRequireProvider|ToolchainCommandDoesNotRequireProvider|ReportsCommandDoesNotRequireProvider|RunsCommandDoesNotRequireProvider|ArtifactsCommandDoesNotRequireProvider|CommandCommandDoesNotRequireProvider|NextCommandDoesNotRequireProvider|StatusCommandDoesNotRequireProvider|ContextCommandDoesNotRequireProvider|UsageCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory)' -count=1 -v
```

Adjacent regression passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|Overview|Capabilities|Mission|Evidence|ReportProjection|ReplayWorkspace|OpenSQLiteReadOnly|WorkflowStatus|ProviderGenerate|GenerateProviderMessage|ToolchainContract|SafetyAuthorization|RuntimeWorker' -count=1 -v
```

Package regression passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1
```

Documentation syntax passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v
```

Full repository validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
```

Build and strict readiness passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
```

Hygiene passed:

```bash
gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
git diff --check
rg -n '[[:blank:]]$' internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go README.md docs/development.md task_plan.md findings.md progress.md docs/progress/milestone-221-ai-chat-local-overview.md
```
