# Milestone 50: Provider Usage Summary Preview

## Summary

Milestone 50 adds a read-only aggregate view over the metadata-only provider usage ledger.

Operators can now run `ouro provider usage <workspace> --summary` to summarize manual provider-preview usage records under `workspace/runtime/model-usage/`. The summary reports record count, provider/model sets, token totals, available cost totals, budget-gate counts, and first/last record timestamps.

## What Changed

- Added app-layer `SummarizeProviderUsageRecords`.
- Reused existing provider usage list/read path safety, including symlink and direct JSON record constraints.
- Added CLI flag `ouro provider usage <workspace> --summary`.
- Kept existing provider usage list and direct-record inspect behavior unchanged.

## Safety and Scope Boundaries

- The summary is read-only and does not write workspace state.
- The summary does not persist prompt text, assistant text, bearer tokens, provider response bodies, or replay events.
- The summary does not contact providers, scoped targets, workers, transports, or tools.
- Manual-preview usage records are not treated as `ouro run` model consumption yet; this is an accounting bridge for later run/replay integration.

## Validation

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'SummarizeProviderUsage|ProviderUsageCommandCanPrintSummary' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'Provider|GenerateProvider|ProviderUsage|RootCommand|SummarizeProviderUsage' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict` printed `workspace: ready`
- `git diff --check`
- `gofmt -l internal/app/provider_usage.go internal/app/provider_usage_test.go cmd/ouro/main.go cmd/ouro/main_test.go`

## Remaining Gaps

The IDE is still not complete. Current effective gaps include binary production payload bundling, fuller managed rule corpus packaging/provenance, replay/run-integrated model accounting and budget enforcement, streaming/tool-call/provider-failover model runtime integration, model capability routing, generation integration into actual run/agent planning, and autonomous distributed worker scheduling beyond bounded/gated audit runs.
