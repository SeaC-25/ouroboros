# Milestone 02: Sisyphus Engine And Frontier

## Implemented
- Added Sisyphus task contracts in `internal/sisyphus/types.go`.
- Added task kinds: `bootstrap`, `reason`, `explore`, and `conclude`.
- Added `Attempt`, `FailureReason`, `MutationHint`, and `TaskResult`.
- Added deterministic engine step in `internal/sisyphus/engine.go`.
- Added tests in `internal/sisyphus/engine_test.go` for new-project bootstrap, open-intent exploration, no-open-intent reasoning, and conclude fallback model types.
- Added Conclude Fallback in `internal/sisyphus/engine.go`.
- Added failure classifications for timeout, parse failure, and worker exit.
- Added partial transcript summarization, reusable lead extraction, and Blackboard write-back through `ConcludeIntent`.
- Added tests in `internal/sisyphus/conclude_test.go` for timed-out attempts, parse failures, worker exit with partial transcript, useful Fact creation, and reusable-lead-only hints.
- Added Frontier candidate path model in `internal/frontier/types.go`.
- Added Breadth / Depth score formula in `internal/frontier/scoring.go`.
- Added priority queue ordering in `internal/frontier/queue.go`.
- Added tests in `internal/frontier/frontier_test.go` for high mission value, exhausted penalty, duplicate penalty, novelty, and highest-score queue popping.
- Added Coverage Matrix in `internal/frontier/coverage.go`.
- Added Web/API, Infra/Internal, and CTF/Lab coverage domains and default dimensions.
- Added discovered/tested/verified/exhausted counters and heatmap buckets.
- Added tests in `internal/frontier/coverage_test.go` for default dimensions, counter updates, and `unknown/shallow/active/verified/exhausted` bucket transitions.

## Not Implemented
- Domain-specific weighting and domain-pack-driven coverage dimensions are deferred to Domain Packs.

## Validation
- `GOPROXY=https://goproxy.cn,direct go test ./internal/sisyphus -v`
- `GOPROXY=https://goproxy.cn,direct go test ./internal/frontier -v`
- `GOPROXY=https://goproxy.cn,direct go test ./...`

## Next Step
- Start Milestone 3 with Task 3.1: Pattern Schema.

## Risks / Decisions
- The first Sisyphus step is deterministic and state-driven: empty state schedules `bootstrap`, open intents schedule `explore`, and otherwise the engine schedules `reason`.
- The engine currently reads `blackboard.ProjectState`; write-back behavior belongs to later orchestrator and task execution layers.
- Conclude Fallback intentionally records a conservative `attempt_summary` Fact and returns only reusable leads from lines prefixed `lead:`, `hint:`, or `reusable:`.
- Frontier scoring currently follows the architecture formula directly and leaves domain-specific weighting for later domain pack integration.
- Coverage Matrix starts with fixed dimensions from the architecture document; additional dimensions should be introduced through Domain Pack metadata later.
