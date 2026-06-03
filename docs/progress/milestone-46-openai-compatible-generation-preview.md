# Milestone 46: OpenAI-Compatible Generation Preview

## Summary

Milestone 46 adds a manual provider generation preview for enabled `openai_compatible` providers.

Operators can now run:

```text
ouro provider generate <workspace> --prompt "Summarize the scoped plan." --system "You are Ouroboros." --max-tokens 128
```

The command calls:

```text
POST <base_url>/chat/completions
```

It sends a bearer token only from `api_key_env`, parses the first assistant message text, and reports prompt, completion, and total token usage.

## Execution Boundary

- This is a manual preview command, not autonomous run integration.
- Default `ouro run` still performs only the provider readiness/model-usage path and does not call chat completions.
- `offline` providers are rejected for generation.
- `base_url` must be absolute `http` or `https` and must not include query, fragment, or userinfo.
- Missing or blank `api_key_env` values fail before HTTP.
- Non-2xx generation responses report provider name and status only; provider response bodies and token values are not included in errors.
- The preview does not execute tools, dispatch workers, contact scoped targets, write reports, persist replay events, mediate tool calls, stream output, perform provider failover, or run model-authored attack planning.

## Validation

Validated during this milestone with:

```text
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestGenerateProviderMessage' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'TestProviderGenerateCommandUsesOpenAICompatibleProvider|TestRootCommandIncludesProductionCommands' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestGenerateProviderMessage|TestRunWorkspaceRejectsOpenAICompatibleProvider(BaseURLUserinfo|InvalidBaseURL|MissingAPIKeyEnv|Non2xxPreflight)|TestRunWorkspacePreflightsOpenAICompatibleProvider' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'Provider|RootCommand' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Provider|GenerateProvider|Model|Replay' -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'Provider|RootCommand' -count=1
git diff --check
gofmt -l internal/app/provider_generation.go internal/app/provider_generation_test.go internal/app/run.go internal/app/run_test.go internal/app/workspace.go cmd/ouro/main.go cmd/ouro/main_test.go
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict
```

The RED runs failed because `GenerateProviderMessage`, the request/result types, and the `provider` CLI command did not exist. After implementation, focused provider and CLI tests passed. A sidecar review found two security hardening items: reject `base_url` userinfo and compact provider/model CLI output. Both were implemented with regression coverage before the full repository test pass.

Strict doctor printed `workspace: ready`; diff hygiene and touched-file formatting checks returned no output.

## Remaining Gaps

The IDE is still not complete. Current effective gaps include binary production payload bundling, fuller managed rule corpus packaging/provenance, streaming/tool-call/provider-failover model runtime integration beyond the manual preview, model capability and cost accounting, generation integration into actual run/agent planning, and autonomous distributed worker scheduling beyond bounded/gated audit runs.
