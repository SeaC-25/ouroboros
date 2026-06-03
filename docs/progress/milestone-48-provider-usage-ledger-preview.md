# Milestone 48: Provider Usage Ledger Preview

## Summary

Milestone 48 adds an explicit metadata-only usage ledger for manual provider previews.

`ouro provider generate` remains non-persistent by default. When operators pass:

```text
ouro provider generate <workspace> --prompt "Summarize scope." --record-usage
```

the command writes a JSON usage record under:

```text
workspace/runtime/model-usage/
```

The new read path is:

```text
ouro provider usage <workspace> [record]
```

It lists all usage records or inspects one direct JSON record by basename or workspace-confined path.

## Execution Boundary

- `--record-usage` is an explicit opt-in write gate after a successful manual preview.
- Default provider preview still writes no workspace state.
- Usage records contain provider/model identity, token counts, optional context window, normalized capabilities, optional estimated cost, source, and timestamp.
- Usage records do not persist prompt text, assistant text, bearer tokens, provider response bodies, or replay events.
- The usage directory rejects symlinks and non-direct JSON record paths.
- This is a ledger seed, not budget enforcement, replay model accounting, provider routing, streaming, tool-call mediation, or autonomous run planning.

## Validation

Validated during this milestone with:

```text
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'ProviderUsage|RecordProviderGenerationUsage|TestProviderGenerateCommandCanRecordUsage' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'Provider|GenerateProvider|ProviderUsage|RootCommand' -count=1
git diff --check
gofmt -l internal/app/provider_usage.go internal/app/provider_usage_test.go cmd/ouro/main.go cmd/ouro/main_test.go
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict
```

The RED run failed because `RecordProviderGenerationUsage`, `ListProviderUsageRecords`, `ReadProviderUsageRecord`, `--record-usage`, and `provider usage` did not exist. After implementation, focused provider usage tests, broader provider regressions, full repository tests, strict doctor, diff hygiene, and formatting checks passed.

Strict doctor printed `workspace: ready`; diff hygiene and touched-file formatting checks returned no output.

## Remaining Gaps

The IDE is still not complete. Current effective gaps include binary production payload bundling, fuller managed rule corpus packaging/provenance, budget enforcement, replay/run-integrated model accounting, streaming/tool-call/provider-failover model runtime integration, model capability routing, generation integration into actual run/agent planning, and autonomous distributed worker scheduling beyond bounded/gated audit runs.
