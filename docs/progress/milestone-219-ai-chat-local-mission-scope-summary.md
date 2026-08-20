# Milestone 219: AI Chat Local Mission/Scope Summary

## Summary

Added local AI dialog commands for mission/scope posture:

- `/mission`
- `/targets`
- `/target-scope`

The commands run before provider generation and return only count/status metadata from `mission.yaml` and `scope.yaml`:

```text
mission: configured=<bool> goal_set=<bool> target_policy=<policy> isolation=<mode> targets=<n> urls=<n> domains=<n> hosts=<n> cidrs=<n> artifacts=<n> challenges=<n> exclusions=<n> stored=false
```

`/scope` remains the existing safety-boundary summary alias for compatibility.

## Safety Boundary

The local summary:

- does not call providers
- does not create `replay/events.db`
- does not write replay, report, model-generation, model-usage, model-tool-plan, compile-preview, worker, or execution artifacts
- does not run tools, dispatch workers, or contact targets
- does not expose mission names, mission goals, raw target values, raw exclusions, workspace paths, provider setup commands, API key values, `Authorization`, or `Bearer` tokens
- is excluded from later provider history together with other local command prompts/responses

## Validation

Focused RED/GREEN passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogMissionCommandDoesNotRequireProvider -count=1 -v
```

Local AI dialog command regression passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(MissionCommandDoesNotRequireProvider|EvidenceCommandDoesNotRequireProvider|WorkersCommandDoesNotRequireProvider|ProvidersCommandDoesNotRequireProvider|SafetyCommandDoesNotRequireProvider|ToolchainCommandDoesNotRequireProvider|ReportsCommandDoesNotRequireProvider|RunsCommandDoesNotRequireProvider|ArtifactsCommandDoesNotRequireProvider|CommandCommandDoesNotRequireProvider|NextCommandDoesNotRequireProvider|StatusCommandDoesNotRequireProvider|ContextCommandDoesNotRequireProvider|UsageCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory)' -count=1 -v
```

Adjacent regression passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|Mission|Evidence|ReportProjection|ReplayWorkspace|OpenSQLiteReadOnly|WorkflowStatus|ProviderGenerate|GenerateProviderMessage|ToolchainContract|SafetyAuthorization|RuntimeWorker' -count=1 -v
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
rg -n '[[:blank:]]$' internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go README.md docs/development.md task_plan.md findings.md progress.md docs/progress/milestone-219-ai-chat-local-mission-scope-summary.md
```

## Completion Status

Completion remains about 99.99%. Direct chat now has local mission/scope posture in addition to local status, context, usage, provider, worker, evidence, runs, reports, toolchain, and safety summaries. True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator evidence review.
