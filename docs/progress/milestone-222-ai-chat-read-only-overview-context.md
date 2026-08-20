# Milestone 222: AI Chat Read-Only Overview Context

## Summary

Extended the workspace-bound AI dialog system prompt so normal provider-backed prompts receive the same safe posture awareness operators can inspect locally.

The system prompt now includes:

- existing workflow context fields
- count/status-only `overview.*` fields for workflow, mission/scope, safety, providers, capabilities, evidence, reports, and workers

Example fields:

```text
overview.status=available
overview.workflow_status=<status>
overview.stage=<stage>
overview.next_gate=<gate>
overview.mission_configured=<bool>
overview.target_policy=<policy>
overview.isolation=<mode>
overview.targets=<n>
overview.safety_ready=<bool>
overview.strict_ready=<bool>
overview.providers_status=<status>
overview.providers=<n>
overview.provider_enabled=<n>
overview.chat=<bool>
overview.tool_planning=<bool>
overview.capabilities_status=<status>
overview.domain_packs=<n>
overview.tools=<n>
overview.evidence_runs=<n>
overview.findings=<n>
overview.reports_current=<bool>
overview.workers_registered=<n>
overview.worker_queue_claims=<n>
```

If overview summarization fails, the prompt emits only:

```text
overview.status=unavailable
overview.error=overview_unavailable
```

## Safety Boundary

The injected overview context:

- does not create replay databases or replay events
- does not write reports, provider usage records, model artifacts, tool artifacts, worker artifacts, or execution artifacts
- does not run health probes, dispatch workers, execute tools, call providers beyond the requested prompt, or contact targets
- does not expose mission names, goal text, target values, exclusions, provider URLs, model names, API-key environment names, API key values, workspace paths, command arrays, manifest paths, payload paths, install paths, report paths, local command transcript text, `Authorization`, or `Bearer` tokens
- remains separate from local command history; local `/overview` output still does not become a user/assistant history message for later provider calls

## Validation

Focused RED first showed the provider system prompt contained only workflow context and lacked `overview.status=available`. Focused GREEN passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogSendsReadOnlyWorkspaceContext -count=1 -v
```

Focused AI dialog context/history regression passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(OverviewCommandDoesNotRequireProvider|ContextCommandDoesNotRequireProvider|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory|LocalCommandsDoNotEnterProviderHistory|HelpCommandDoesNotRequireProvider)' -count=1 -v
```

Adjacent regression passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|Overview|Context|Capabilities|Mission|Evidence|ReportProjection|ReplayWorkspace|OpenSQLiteReadOnly|WorkflowStatus|ProviderGenerate|GenerateProviderMessage|ToolchainContract|SafetyAuthorization|RuntimeWorker' -count=1 -v
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
rg -n '[[:blank:]]$' internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go README.md docs/development.md task_plan.md findings.md progress.md docs/progress/milestone-222-ai-chat-read-only-overview-context.md
```
