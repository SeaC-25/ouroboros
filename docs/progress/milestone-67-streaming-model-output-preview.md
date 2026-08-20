# Milestone 67: Streaming Model Output Preview

Date: 2026-06-05

## Summary

Milestone 67 narrows the model-runtime gap by adding an explicit streaming preview mode for OpenAI-compatible provider generation.

The default path remains non-streaming. Streaming is requested only when an operator passes `ouro provider generate --stream`, `ouro run --provider-generate --provider-generate-stream`, or when app callers set the matching request/options fields.

When streaming is enabled, the provider request sets `stream=true` and asks for usage metadata with `stream_options.include_usage=true`. SSE content chunks are aggregated into the same in-memory result used by the existing generation path. Persisted state stores only stream metadata: completion status, chunk count, stream digest, and bounded stream preview.

## Surfaces

- App:
  - `ProviderGenerationRequest` accepts `Stream`.
  - `RunOptions` accepts `ProviderGenerationStream`.
  - `ProviderGenerationResult` and `ModelGenerationArtifact` carry `streaming`, `stream_completed`, `stream_chunk_count`, `stream_digest`, and `stream_preview`.
  - Run-integrated `model_generation` events include the same safe stream metadata.
- CLI:
  - `ouro provider generate <workspace> --stream` requests streamed manual generation.
  - `ouro run <workspace> --provider-generate --provider-generate-stream` requests streamed run-integrated planner generation.
  - `ouro provider generate` and `ouro provider generations` print safe stream metadata when present.
- TUI:
  - Replay-derived `run model generation:` rows and persisted `model generation artifact:` rows render safe stream metadata.
  - Runtime Tabs Inspector details remain read-only and include stream metadata in the output field.

## Boundary

- Default provider generation remains non-streaming.
- Streaming does not create per-chunk replay events or persist raw chunks.
- Prompt text, full assistant text, raw stream chunks, bearer tokens, provider request/response bodies, raw provider errors, and executable model instructions must not be persisted.
- Budget enforcement remains fail-closed when estimated cost metadata is required but unavailable.
- This is not model-mediated tool-call execution, provider load balancing, autonomous multi-round planning, worker dispatch, another tool round, or scoped target contact.

## Validation

RED was confirmed before implementation with:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'Stream|Streaming|ProviderGeneration|ModelGeneration' -count=1`

Focused GREEN and touched-package regression passed with:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/eventlog ./cmd/ouro ./internal/tui -run 'Stream|Streaming|ProviderGeneration|ModelGeneration|ModelUsagePreviews' -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/eventlog ./internal/tui ./cmd/ouro -count=1`

Full validation after documentation synchronization passed with:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `git diff --check`
- `gofmt -l internal/app/provider_generation.go internal/app/run.go internal/app/planner_loop.go internal/app/model_generation_artifact.go internal/app/tui_snapshot.go internal/eventlog/replay.go internal/tui/replay.go internal/tui/runtime_tabs.go internal/app/provider_generation_test.go internal/app/run_test.go internal/app/tui_snapshot_test.go internal/eventlog/replay_test.go internal/tui/replay_test.go internal/tui/runtime_tabs_test.go cmd/ouro/main.go cmd/ouro/main_test.go`
