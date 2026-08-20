# Milestone 66: Provider Capability Routing Preview

Date: 2026-06-05

## Summary

Milestone 66 narrows the remaining model-runtime gap by adding an explicit required-capability routing preview for configured providers.

The contract is conservative: provider capability routing is used only when an operator supplies required capabilities through `ouro provider generate --required-capability` or `ouro run --provider-generate-required-capability`, or when app callers populate the matching request/options fields. Without required capabilities, provider selection keeps the existing first-enabled or explicit failover behavior.

When required capabilities are present, providers whose declared `capabilities` do not satisfy the request are skipped before preflight or generation HTTP. Skipped providers are recorded separately from failed providers, using only provider name plus the fixed `missing_required_capabilities` reason.

## Surfaces

- App:
  - `ProviderGenerationRequest` accepts `RequiredCapabilities`.
  - `RunOptions` accepts `ProviderGenerationRequiredCapabilities`.
  - `GenerateProviderMessage` and provider preflight filter enabled providers by declared local capabilities.
  - `ProviderGenerationResult`, `ProviderRuntime`, and `ModelGenerationArtifact` carry `required_capabilities`, `capability_match`, and `skipped_providers`.
- CLI:
  - `ouro provider generate <workspace> --required-capability <capability>` requests capability routing for manual previews.
  - `ouro run <workspace> --provider-generate --provider-generate-required-capability <capability>` requests capability routing for the run-integrated planner generation gate.
  - `ouro provider generate` and `ouro provider generations` print safe capability-route metadata when present.
- TUI:
  - Workspace-bound Model Runtime appends `model generation capability route:` rows for persisted run generation artifacts.

## Boundary

- Default no-required-capability behavior is unchanged.
- Capability skipped providers are not failed providers.
- Skipped providers are not contacted over HTTP.
- Persisted capability-route metadata is limited to required capability labels, match status, provider names, and fixed skip reasons.
- Prompt text, full assistant text, bearer tokens, provider request/response bodies, raw provider errors, and executable model instructions must not be persisted.
- This is not provider load balancing, streaming output, model-mediated tool-call execution, autonomous multi-round planning, worker dispatch, another tool round, or scoped target contact.

## Validation

RED was confirmed before implementation with:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'RequiredCapabilities|RequiredCapability|RoutesByRequiredCapabilities|ShowsModelGenerationReviewArtifacts' -count=1`

Focused GREEN and touched-surface regression passed with:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'RequiredCapabilities|RequiredCapability|RoutesByRequiredCapabilities|ShowsModelGenerationReviewArtifacts' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Provider|ModelGeneration|RunWorkspaceProviderGeneration|RunWorkspaceProviderPreflight' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'Provider|RunCommandPassesProviderGenerationOptions|RunCommandRejects|RootCommand' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app -run 'ModelGeneration|ProviderGeneration|RuntimeTabs|NewTUIModelForWorkspaceShowsModelGenerationReviewArtifacts' -count=1`

Full validation after documentation synchronization passed with:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `git diff --check`
- `gofmt -l internal/app/provider_generation.go internal/app/run.go internal/app/planner_loop.go internal/app/model_generation_artifact.go internal/app/tui_snapshot.go internal/app/provider_generation_test.go internal/app/run_test.go internal/app/tui_snapshot_test.go cmd/ouro/main.go cmd/ouro/main_test.go`
