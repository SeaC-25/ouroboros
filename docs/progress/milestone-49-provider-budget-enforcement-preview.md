# Milestone 49: Provider Budget Enforcement Preview

## Summary

Milestone 49 adds an explicit manual provider-preview budget gate for `openai_compatible` generation previews.

Operators can now pass `ouro provider generate <workspace> --max-estimated-cost-usd <n>` to require the actual locally estimated cost of a completed provider response to stay within the supplied USD limit before assistant text is printed or a usage record is written. When pricing metadata is missing, or the estimate exceeds the limit, the preview fails closed and writes no metadata ledger record.

## What Changed

- Added app-layer budget evaluation for `ProviderGenerationResult`.
- Extended `RecordProviderGenerationUsage` so successful budgeted previews persist only `budget_enforced`, `budget_status`, and `budget_max_estimated_cost_usd` alongside the existing metadata-only usage fields.
- Added CLI flag `--max-estimated-cost-usd` to `ouro provider generate`.
- Budget rejection happens after the provider response returns and local cost estimation is available, but before CLI assistant text output and before `--record-usage` writes under `workspace/runtime/model-usage/`.
- Missing pricing metadata fails closed when a budget flag is set, because the preview cannot prove the result is within budget.

## Safety and Scope Boundaries

- The budget gate does not persist prompts, assistant text, bearer tokens, provider response bodies, or replay events.
- The budget gate does not contact scoped targets, execute tools, dispatch workers, write reports, mediate tool calls, stream model output, perform provider failover, route by model capability, or attach model generation to `ouro run`.
- This is an acceptance/recording gate for manual previews, not a pre-spend reservation. The provider HTTP request has already completed when actual usage is known.

## Validation

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'Provider.*Budget|RecordProviderGenerationUsage.*Budget|ProviderGenerateCommand.*Budget' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'Provider|GenerateProvider|ProviderUsage|RootCommand' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict` printed `workspace: ready`
- `git diff --check`
- `gofmt -l internal/app/provider_usage.go internal/app/provider_usage_test.go cmd/ouro/main.go cmd/ouro/main_test.go`

## Remaining Gaps

The IDE is still not complete. Current effective gaps include binary production payload bundling, fuller managed rule corpus packaging/provenance, replay/run-integrated model accounting and budget enforcement, streaming/tool-call/provider-failover model runtime integration, model capability routing, generation integration into actual run/agent planning, and autonomous distributed worker scheduling beyond bounded/gated audit runs.
