# Milestone 218: AI Chat Local Evidence Summary

## Status

Complete on 2026-06-10.

## What Changed

- Added local AI dialog commands `/evidence`, `/findings`, and `/graph`.
- The commands read `replay/events.db` through `eventlog.OpenSQLiteReadOnly` and do not create the event DB when it is absent.
- The latest replay run is projected into an evidence graph and in-memory report summary.
- The response returns count-only metadata: runs, latest-run availability, events, facts, intents, hints, nodes, edges, findings, surfaces, artifacts, verified findings, paths, evidence refs, exhausted areas, and suggestions.
- `/help` now lists `/evidence`.
- Local command history isolation now excludes `/evidence` prompts and evidence summary responses from later provider requests.

## Safety Boundary

The evidence summary is local and metadata-only. It does not call providers, mutate replay, write report projections, create model usage or generation artifacts, create model tool-plan previews, create compile-preview reviews or execution dispatches, dispatch workers, execute tools, or contact targets.

The response intentionally avoids run ids, fact ids, fact statements, URLs, hosts, artifact references, report paths, workspace paths, provider setup commands, API keys, Authorization headers, and Bearer tokens.

## Validation

- RED/GREEN focused command test:
  `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogEvidenceCommandDoesNotRequireProvider -count=1 -v`
- Local AI dialog command regression:
  `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(EvidenceCommandDoesNotRequireProvider|WorkersCommandDoesNotRequireProvider|ProvidersCommandDoesNotRequireProvider|SafetyCommandDoesNotRequireProvider|ToolchainCommandDoesNotRequireProvider|ReportsCommandDoesNotRequireProvider|RunsCommandDoesNotRequireProvider|ArtifactsCommandDoesNotRequireProvider|CommandCommandDoesNotRequireProvider|NextCommandDoesNotRequireProvider|StatusCommandDoesNotRequireProvider|ContextCommandDoesNotRequireProvider|UsageCommandDoesNotRequireProvider|HelpCommandDoesNotRequireProvider|LocalCommandsDoNotEnterProviderHistory|SendsReadOnlyWorkspaceContext|SendsCurrentSessionHistory)' -count=1 -v`
- Adjacent regression:
  `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|Evidence|ReportProjection|ReplayWorkspace|OpenSQLiteReadOnly|WorkflowStatus|ProviderGenerate|GenerateProviderMessage|ToolchainContract|SafetyAuthorization|RuntimeWorker' -count=1 -v`
- Package regression:
  `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1`
- Documentation syntax:
  `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`
- Full repository:
  `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- Build:
  `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- Strict readiness:
  `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`

## Completion Note

This improves the direct chat IDE toward a security-operator Codex/Claude Code analogue by giving operators a local, safe evidence posture command inside the conversation. The overall IDE target remains just short of 100% until real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator evidence review are complete.
