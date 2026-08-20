# Milestone 65: Provider Routing Failover Preview

Date: 2026-06-04

## Summary

Milestone 65 narrows the model-runtime gap by adding an explicit provider routing and failover preview.

The contract is conservative: existing workspaces continue to use first-enabled provider selection unless `providers.yaml` opts into `routing.strategy: enabled_order_failover`. In failover mode, run preflight and provider generation try enabled providers in file order until one succeeds or all fail.

Successful manual previews, run-integrated model-generation replay events, review artifacts, CLI output, and workspace-bound Model Runtime rows now expose only review-safe route metadata: strategy, attempt count, failover flag, selected provider, and failed provider names. Aggregate failover errors use provider names plus status/category summaries rather than raw provider errors.

## Surfaces

- Provider config:
  - `routing.strategy: first_enabled` or omitted keeps existing behavior.
  - `routing.strategy: enabled_order_failover` enables ordered failover.
  - Unsupported strategies fail validation.
- App:
  - `GenerateProviderMessage` supports ordered generation failover.
  - `RunWorkspaceWithOptions` provider preflight supports ordered failover.
  - `ProviderGenerationResult`, `ProviderRuntime`, and `ModelGenerationArtifact` carry route metadata.
- CLI/TUI:
  - `ouro provider generate` prints route metadata.
  - `ouro provider generations` list/read output prints route metadata from review artifacts.
  - Workspace-bound Model Runtime appends compact `model generation route:` rows for persisted artifacts.

## Boundary

- Default provider selection remains first-enabled.
- Failover is explicit in provider config, not inferred from multiple enabled providers.
- Success metadata includes routing strategy, attempt count, selected provider, failover flag, and failed provider names.
- Prompt text, full assistant text, bearer tokens, provider request/response bodies, raw provider errors, and executable model instructions must not be persisted.
- Budget enforcement, optional metadata-only usage ledger writes, and model-generation review artifacts still happen only after a successful generation result from the selected provider.
- This is not provider load balancing, capability routing, streaming output, model-mediated tool-call execution, autonomous multi-round planning, worker dispatch, another tool round, or scoped target contact.

## Validation

Focused RED/GREEN and touched-surface validation passed:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestGenerateProviderMessage(FailsOverEnabledProvidersWhenConfigured|KeepsFirstEnabledDefaultWithoutFailover)|TestRunWorkspaceProviderPreflightFailsOverWhenConfigured' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceShowsModelGenerationReviewArtifacts' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'TestProviderGenerateCommandPrintsFailoverRoutingMetadata|TestProviderGenerationsCommandListsAndReadsReviewArtifacts' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestGenerateProviderMessage(FailoverAllFailuresSanitizesAggregateError|FailsOverEnabledProvidersWhenConfigured|KeepsFirstEnabledDefaultWithoutFailover)|TestRunWorkspaceProviderGeneration(FailsOverAndRecordsRoutingMetadata|FailoverFailureDoesNotLeakProviderBodies)|TestRunWorkspaceProviderPreflightFailsOverWhenConfigured' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'Provider|ProviderGeneration|ProviderGenerate|ProviderGenerations|ModelGeneration|RuntimeTabs|NewTUIModelForWorkspaceShowsModelGenerationReviewArtifacts|DoesNotRequestReadOnlyRuntimeActions' -count=1 -v`

Full repository validation after documentation synchronization passed with:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `git diff --check`
- `gofmt -l internal/app/workspace.go internal/app/run.go internal/app/provider_generation.go internal/app/model_generation_artifact.go internal/app/planner_loop.go internal/app/tui_snapshot.go internal/app/provider_generation_test.go internal/app/run_test.go internal/app/tui_snapshot_test.go cmd/ouro/main.go cmd/ouro/main_test.go`
