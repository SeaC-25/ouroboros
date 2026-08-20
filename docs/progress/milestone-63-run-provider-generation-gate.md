# Milestone 63: Run Provider Generation Gate

Date: 2026-06-04

## Summary

This milestone adds the explicit run-integrated provider generation gate for the `closed-loop-planner-preview` stage. Default `ouro run` remains unchanged: it keeps the milestone 62 metadata-only model usage accounting preview and does not call provider generation unless an operator opts in through `ouro run --provider-generate` / related `--provider-generate-*` CLI flags or matching `RunOptions`.

When enabled, the planner stage may call the configured provider once to enrich the next-run planner review. Replay persists only generation metadata: provider/model identity, operation and stage, text digest, bounded short preview, token usage, optional estimated cost, budget status, and ledger write metadata. Prompt text, full assistant text, bearer tokens, provider response bodies, raw provider errors, and complete provider response text are not persisted verbatim.

Optional usage ledger writes reuse the existing metadata-only `workspace/runtime/model-usage/` record boundary. A run generation ledger record includes provider/model identity, token counts, optional cost estimate, and budget decision only; it does not store prompts, complete provider responses, or assistant text.

## Boundary

- Default `ouro run` still does not call provider generation.
- Run-integrated generation requires explicit `--provider-generate` flags or matching `RunOptions`.
- Budget enforcement fails closed before writing optional ledger records and before persisting successful replay `model_generation` metadata.
- Provider response text is never persisted verbatim; replay stores only a digest and bounded short preview.
- Replay generation metadata is not a tool-call channel and cannot execute provider-suggested commands.
- The planner performs at most one explicit provider generation call for the preview stage; there is no autonomous multi-round model planning loop.
- Provider routing, provider failover, streaming model output, model-mediated tool-call execution, worker dispatch, additional tool execution rounds, and scoped target contact remain outside this milestone.

## Validation

- `go test ./internal/app -run 'TestRunWorkspace(DoesNotGenerateProviderMessageByDefault|CanGenerateProviderMessageWithBudgetAndUsageRecord|ProviderGenerationBudgetFailureDoesNotRecordSuccessOrUsage|ProviderGenerationRequiresPrompt)' -count=1 -v`
- `go test ./cmd/ouro -run 'TestRunCommand(ProviderPromptDoesNotEnableGenerationWithoutGate|RejectsProviderGenerateWithoutPrompt|RejectsNegativeProviderGenerationLimits|PassesProviderGenerationOptions|CanGenerateProviderMessageWithBudgetAndUsageRecord|ProviderGenerateRequiresPrompt)' -count=1 -v`
- `go test ./internal/eventlog -run 'TestBuildReplaySnapshotProjectsModelGenerationAsActualUsage' -count=1 -v`
- `go test ./internal/tui -run 'TestModelWithReplaySnapshotShowsProviderGenerationRows' -count=1 -v`

These tests prove default `ouro run` does not call provider generation; opt-in generation calls the provider once; over-budget generation fails before usage records or successful replay `model_generation` events; replay stores provider/model identity, digest, bounded preview, token/cost fields, budget status, and ledger write metadata without prompts, full assistant text, bearer tokens, or provider bodies; and optional ledger writes reuse metadata-only usage records.
