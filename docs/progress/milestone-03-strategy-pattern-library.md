# Milestone 03: Strategy Pattern Library

## Implemented
- Added graph pattern schema in `internal/patterns/types.go`.
- Added YAML loader and validation in `internal/patterns/loader.go`.
- Added built-in pattern files under `configs/patterns/`: `ralphloop.yaml`, `explore_plan_execute.yaml`, and `ant_colony.yaml`.
- Added tests in `internal/patterns/loader_test.go` for built-in loading, missing node references, and required terminal result nodes.
- Added deterministic pattern evaluator in `internal/patterns/evaluator.go`.
- Added evaluator tests in `internal/patterns/evaluator_test.go` for RalphLoop retry, Explore-Plan-Execute progression, Ant Colony deposit, and deterministic edge ordering.

## Not Implemented
- Runtime integration with Sisyphus and Agent Swarm is not implemented yet.

## Validation
- `GOPROXY=https://goproxy.cn,direct go test ./internal/patterns -v`
- `GOPROXY=https://goproxy.cn,direct go test ./...`

## Next Step
- Start Milestone 4 with Task 4.1: Agent Contracts.

## Risks / Decisions
- Pattern schema is intentionally small: nodes, edges, conditions, max steps, and metadata. Domain-specific execution logic should stay outside the schema.
- Built-in patterns are data files so future domain packs can reference or override them without changing Go code.
- Evaluator transition selection is deterministic: it scans edges in YAML order and picks the first edge whose condition result is true. The `always` condition is treated as true.
