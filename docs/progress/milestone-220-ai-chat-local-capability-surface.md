# Milestone 220: AI Chat Local Capability Surface

## Summary

Added local AI dialog commands for capability-surface posture:

- `/capabilities`
- `/packs`
- `/domains`

The commands run before provider generation and return only count/status metadata from built-in domain packs, compiler semantic actions, workspace manifests, the built-in tool catalog, workspace managed bundles, parser and strategy declarations, and target-kind/category coverage:

```text
capabilities: status=<status> domains=<n> domain_packs=<n> pack_capabilities=<n> parsers=<n> strategies=<n> semantic_actions=<n> semantic_capabilities=<n> manifests=<n> manifest_capabilities=<n> manifest_dependencies=<n> tools=<n> tool_capabilities=<n> tool_categories=<n> target_kinds=<n> managed_bundles=<n> managed_installed=<n> managed_blocked=<n> patterns=<n> errors=<n> stored=false
```

## Safety Boundary

The local summary:

- does not call providers
- does not create `replay/events.db`
- does not write replay, report, model-generation, model-usage, model-tool-plan, compile-preview, worker, or execution artifacts
- does not run health probes, execute tools, dispatch workers, or contact targets
- does not expose tool names, command arrays, manifest paths, payload paths, install paths, workspace paths, provider setup commands, API key values, `Authorization`, or `Bearer` tokens
- is excluded from later provider history together with other local command prompts/responses

## Validation

Focused RED/GREEN passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogCapabilitiesCommandDoesNotRequireProvider -count=1 -v
```

Local AI dialog command regression passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(CapabilitiesCommandDoesNotRequireProvider|MissionCommandDoesNotRequireProvider|EvidenceCommandDoesNotRequireProvider|WorkersCommandDoesNotRequireProvider|ProvidersCommandDoesNotRequireProvider|SafetyCommandDoesNotRequireProvider|ToolchainCommandDoesNotRequireProvider|ReportsCommandDoesNotRequireProvider|RunsCommandDoesNotRequireProvider|ArtifactsCommandDoesNotRequireProvider|CommandCommandDoesNotRequireProvider|NextCommandDoesNotRequireProvider|StatusCommandDoesNotRequireProvider|ContextCommandDoesNotRequireProvider|UsageCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory)' -count=1 -v
```

Adjacent regression passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|Capabilities|Mission|Evidence|ReportProjection|ReplayWorkspace|OpenSQLiteReadOnly|WorkflowStatus|ProviderGenerate|GenerateProviderMessage|ToolchainContract|SafetyAuthorization|RuntimeWorker' -count=1 -v
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
rg -n '[[:blank:]]$' internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go README.md docs/development.md task_plan.md findings.md progress.md docs/progress/milestone-220-ai-chat-local-capability-surface.md
```
