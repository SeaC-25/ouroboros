# Milestone 47: Provider Capability And Cost Preview

## Summary

Milestone 47 adds optional provider metadata for manual `openai_compatible` generation previews.

`providers.yaml` can now declare:

```yaml
context_window_tokens: 128000
input_price_per_million_tokens: 2.50
output_price_per_million_tokens: 10.00
capabilities:
  - chat
  - json_mode
```

`ouro provider generate` still calls `POST <base_url>/chat/completions` only for an explicit manual preview, but it now prints configured context-window metadata, normalized capability labels, and an estimated cost derived from provider-reported prompt/completion token usage when pricing metadata is configured.

## Execution Boundary

- This is metadata and cost transparency for the manual preview command only.
- Default `ouro run` remains on the readiness/model-usage path and does not call chat completions.
- Pricing values are local operator metadata, not billing authority.
- Missing pricing suppresses `estimated_cost_usd` output instead of printing a misleading zero-cost estimate.
- Negative context-window or pricing metadata fails provider config validation before HTTP.
- Capabilities are trimmed, de-duplicated, and sorted for stable CLI output.
- The preview still does not execute tools, dispatch workers, contact scoped targets, write reports, persist replay events, mediate tool calls, stream output, perform provider failover, enforce budgets, or run model-authored attack planning.

## Validation

Validated during this milestone with:

```text
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/modelruntime ./internal/app ./cmd/ouro -run 'EstimateTokenCost|TestGenerateProviderMessageReportsMetadataAndEstimatedCost|TestRunWorkspaceRejectsOpenAICompatibleProviderNegativeCostMetadata|TestProviderGenerateCommandPrintsMetadataAndEstimatedCost' -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/modelruntime ./internal/app ./cmd/ouro -run 'Pricing|EstimateTokenCost|Provider|GenerateProvider|Model|Replay|RootCommand' -count=1
git diff --check
gofmt -l internal/modelruntime/pricing.go internal/modelruntime/pricing_test.go internal/app/workspace.go internal/app/provider_generation.go internal/app/provider_generation_test.go internal/app/run_test.go cmd/ouro/main.go cmd/ouro/main_test.go
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict
```

The RED run failed because `EstimateTokenCost`, provider metadata result fields, CLI metadata output, and provider metadata validation did not exist. After implementation, the focused tests, related provider/modelruntime regression, full repository test suite, strict doctor, diff hygiene, and formatting checks passed.

Strict doctor printed `workspace: ready`; diff hygiene and touched-file formatting checks returned no output.

## Remaining Gaps

The IDE is still not complete. Current effective gaps include binary production payload bundling, fuller managed rule corpus packaging/provenance, persistent model usage ledgers and budget enforcement, streaming/tool-call/provider-failover model runtime integration, model capability routing, generation integration into actual run/agent planning, and autonomous distributed worker scheduling beyond bounded/gated audit runs.
