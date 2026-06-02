# Milestone 05: Model Runtime Monitor

## Implemented
- Added usage records and aggregates in `internal/modelruntime/types.go`.
- Added in-memory usage ledger in `internal/modelruntime/ledger.go`.
- Ledger tracks provider, model, role, agent, run, input/output tokens, cache read/write tokens, cost, latency, and failures.
- Added tests in `internal/modelruntime/ledger_test.go` for aggregation by agent, role, run, and model, including cost and failure counts.
- Added Context Health in `internal/modelruntime/context_health.go`.
- Context Health tracks used tokens, max context window, percent used, pinned objects, next threshold, and health state.
- Added threshold behavior: 70% rolling summary, 85% compact attempts, and 95% checkpoint.
- Added tests in `internal/modelruntime/context_health_test.go` for each threshold and token percentage calculation.

## Not Implemented
- Provider adapter integration is not implemented yet.
- TUI model runtime panels are not implemented yet.

## Validation
- `GOPROXY=https://goproxy.cn,direct go test ./internal/modelruntime -v`
- `GOPROXY=https://goproxy.cn,direct go test ./...`

## Next Step
- Start Milestone 6 with Task 6.1: Toolchain Manifest.

## Risks / Decisions
- The ledger is currently in-memory. Durable model usage events should flow through Event Log integration later.
- Cost is recorded as provided by adapters; provider-specific pricing calculation can be added in provider adapters.
- Context Health thresholds follow the architecture document exactly and keep pinned objects as immutable snapshots.
