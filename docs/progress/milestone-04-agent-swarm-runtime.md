# Milestone 04: Agent Swarm Runtime

## Implemented
- Added Agent contract types in `internal/swarm/types.go`.
- Added required roles: planner, web-api, infra, ctf, verifier, evidence, reporter, context, and graph.
- Added Agent states, task, result, thread, structured output, proposal type, and proposal payload contracts.
- Added structured output validation in `internal/swarm/output_contracts.go`.
- Added tests in `internal/swarm/contracts_test.go` for required roles, accepted proposal types, rejected unsupported proposal types, and invalid JSON payload rejection.
- Added provider-backed driver interface in `internal/swarm/driver.go`.
- Added Swarm Orchestrator in `internal/swarm/orchestrator.go`.
- Orchestrator validates agent output, writes Fact/Intent/Hint/Finding/Mutation proposals to Blackboard, and exposes current Evidence Graph projection.
- Added Blackboard Hint append support in `internal/blackboard/store.go`.
- Added progress aggregation for complete, blocked, failed, queued, and running states.
- Added tests in `internal/swarm/orchestrator_test.go` for validated write-back, malformed output rejection without state mutation, Evidence Graph projection, and progress aggregation.
- Added Timeline Lanes in `internal/swarm/timeline.go`.
- Added lane mapping for mission, agent, observer, blackboard, toolchain, and submission-report events.
- Added tests in `internal/swarm/timeline_test.go` for lane assignment, chronological ordering, and payload preservation.

## Not Implemented
- Runtime integration between the timeline projection and the TUI is not implemented yet.

## Validation
- `GOPROXY=https://goproxy.cn,direct go test ./internal/swarm -v`
- `GOPROXY=https://goproxy.cn,direct go test ./internal/blackboard -v`
- `GOPROXY=https://goproxy.cn,direct go test ./...`

## Next Step
- Start Milestone 5 with Task 5.1: Usage Ledger.

## Risks / Decisions
- Structured output uses proposal envelopes so agents cannot request direct tool execution or arbitrary state mutation through this contract.
- Payloads are validated as JSON at the contract layer; semantic conversion into Blackboard objects belongs to the orchestrator.
- Deterministic drivers exist only in tests. Production code defines the driver interface but does not include fake providers.
- Timeline lane projection is pure and works from Event Log events; persistence and replay integration remain in the replay milestone.
