# Milestone 64: Run Model Generation Review Artifact

Date: 2026-06-04

## Summary

Milestone 64 makes the explicit run-integrated provider generation gate reviewable as durable IDE state.

When `RunWorkspaceWithOptions` runs with `EnableProviderGeneration` or `ouro run --provider-generate`, the closed-loop planner generation stage now writes one metadata-only JSON artifact under `workspace/runtime/model-generations/` after provider generation and budget enforcement succeed. Default runs write no model-generation review artifact, and over-budget generation failures fail before review artifact writes.

The artifact contains only replay-safe review metadata: schema version, artifact id, run id, source/stage, closed-loop planner agent identity, status/mode, provider/model identity, token counters, optional context/capability/cost metadata, budget status, ledger-write state, text digest, bounded preview, latency, and `assistant_text_persisted=false`.

## Surfaces

- App API:
  - `RecordModelGenerationArtifact`
  - `ListModelGenerationArtifacts`
  - `ReadModelGenerationArtifact`
- CLI:
  - `ouro provider generations <workspace>`
  - `ouro provider generations <workspace> <artifact>`
- TUI:
  - Workspace-bound Model Runtime appends `model generation artifact:` rows for persisted review artifacts.
  - Replay-derived `run model generation:` rows and persisted artifact rows both open read-only Inspector details through `d`.
  - `r` and `x` remain unsupported/read-only for model generation review rows.

## Boundary

- No default `ouro run` generation artifact.
- No artifact on budget failure.
- No prompt text, full assistant text, bearer tokens, provider request/response bodies, raw provider errors, or executable model instructions are persisted.
- List/read paths are workspace-confined direct JSON artifact reads and reject traversal, nested paths, non-JSON paths, symlinked directories, and symlinked files.
- Review artifacts do not mutate replay events or usage records after creation.
- CLI/TUI review paths do not contact providers, dispatch workers/tools, execute model output, execute another tool round, route/fail over providers, stream model output, or contact scoped targets.

## Validation

Focused RED/GREEN checks were run during implementation:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRunWorkspace(CanGenerateProviderMessageWithBudgetAndUsageRecord|ProviderGenerationBudgetFailureDoesNotRecordSuccessOrUsage|DoesNotGenerateProviderMessageByDefault)' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'TestProviderGenerationsCommandListsAndReadsReviewArtifacts' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceShowsModelGenerationReviewArtifacts' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestModelUpdate(ShowsModelGenerationArtifactDetailsReadOnly|DoesNotExecuteModelGenerationArtifact|DoesNotRequestReadOnlyRuntimeActions|ShowsRunModelGenerationDetailsReadOnly|DoesNotExecuteRunModelGeneration)' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'ModelGeneration|ProviderGenerations|ProviderGeneration|RunWorkspace(CanGenerateProviderMessageWithBudgetAndUsageRecord|ProviderGenerationBudgetFailureDoesNotRecordSuccessOrUsage|DoesNotGenerateProviderMessageByDefault)|NewTUIModelForWorkspaceShowsModelGenerationReviewArtifacts|RuntimeTabs|DoesNotRequestReadOnlyRuntimeActions' -count=1 -v`

Full repository validation is tracked in `progress.md` for the final milestone close-out.

## Remaining Gaps

The IDE is still not complete. Remaining gaps include binary production payload bundling, fuller managed rule corpus packaging/provenance, provider routing/failover, streaming model output, model capability routing, model-mediated tool-call planning under explicit safety gates, and autonomous distributed worker scheduling beyond bounded/gated audit runs.
