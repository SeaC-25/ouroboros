# Milestone 52: Provider Usage Summary Hosted Replay Workbench Preview

## Summary

The hosted replay workbench now exposes the existing metadata-only manual provider-preview usage summary as read-only review data.

This extends the provider visibility chain:

- Milestone 50: `ouro provider usage --summary`
- Milestone 51: workspace-bound TUI Model Runtime summary rows
- Milestone 52: hosted replay workbench summary endpoint and HTML preview

## Implemented

- Added `GET /api/provider-usage-summary` to the hosted replay workbench.
- Embedded `window.OURO_REPLAY_PROVIDER_USAGE_SUMMARY` in the hosted HTML page.
- Added a compact local provider usage summary section to the hosted page.
- Added `provider_usage_summary` to replay workbench session endpoint metadata.
- Added the same endpoint to saved/previewed replay workbench session artifacts and their CLI/TUI endpoint allowlists.
- Reused `SummarizeProviderUsageRecords` over `workspace/runtime/model-usage/`.
- Added JSON tags to `ProviderUsageSummary` so hosted output is stable snake_case JSON.
- Added handler, CLI, and TUI coverage for structured JSON, HTML embedding, read-only POST rejection, spaces and `|` in provider/model names, saved session endpoint metadata, sensitive text non-leakage, and no replay event or usage-record mutation.

## Boundaries

This is a hosted replay workbench visibility bridge over manual-preview usage metadata only.

It does not:

- write provider usage records
- create browser usage-record write routes
- mutate replay events
- merge manual-preview records into replay `model_usage`
- contact providers
- dispatch workers or tools
- contact targets
- persist prompt text, assistant text, bearer tokens, provider bodies, or generated text
- integrate provider generation, usage accounting, or budget enforcement into `ouro run`

## Validation

Passed before implementation:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
```

Passed focused regression after implementation:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro ./internal/app -run 'ProviderUsage|ReplayWorkbench(CommandServesHTMLAndReplayJSON|HandlerServesProviderUsageSummaryReadOnly|HandlerRejectsPostOnAllReadOnlyRoutesByDefault)|RootCommand' -count=1 -v
```

Passed post-documentation validation:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro ./internal/app -run 'ProviderUsage|ReplayWorkbench(CommandServesHTMLAndReplayJSON|HandlerServesProviderUsageSummaryReadOnly|HandlerRejectsPostOnAllReadOnlyRoutesByDefault)|RootCommand' -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict
git diff --check
gofmt -l cmd/ouro/main.go cmd/ouro/main_test.go internal/app/provider_usage.go internal/app/replay_workbench_session.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
```

Passed review-fix focused regression:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro ./internal/app ./internal/tui -run 'ProviderUsage|ReplayWorkbench(CommandServesHTMLAndReplayJSON|HandlerServesProviderUsageSummaryReadOnly|HandlerRejectsPostOnAllReadOnlyRoutesByDefault|SessionCommandWritesAndListsMetadataOnlyArtifacts|SessionSelectionCommandsSelectReadAndClearMetadataOnly)|NewTUIModelForWorkspace(ShowsProviderUsage|ProviderUsage|ShowsReplayWorkbenchSessionArtifacts)|RootCommand|RuntimeTabs' -count=1
```

Passed review-fix full validation:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict
git diff --check
gofmt -l cmd/ouro/main.go cmd/ouro/main_test.go internal/app/provider_usage.go internal/app/replay_workbench_session.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
```

## Remaining Gaps

Current effective gaps remain binary production payload bundling, fuller managed rule corpus packaging/provenance, replay/run-integrated model accounting and budget enforcement, streaming/tool-call/provider-failover model runtime integration, model capability routing, generation integration into actual run/agent planning, and autonomous distributed worker scheduling beyond bounded/gated audit runs.
