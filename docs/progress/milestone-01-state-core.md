# Milestone 01: State Core

## Implemented
- Added Goal Stack contracts in `internal/goal/types.go`.
- Added validation in `internal/goal/validate.go` for non-empty mission, non-empty success criteria, non-negative weights, and normalized weight totals.
- Added tests in `internal/goal/goal_test.go` for a valid stack, empty mission, unnormalized weights, and missing success criteria.
- Added Blackboard Protocol contracts in `internal/blackboard/types.go` for `Fact`, `Intent`, `Hint`, `Lease`, and `ProjectState`.
- Added in-memory blackboard store in `internal/blackboard/store.go`.
- Implemented append-only Fact snapshots and Intent lifecycle transitions: `Open`, `Claimed`, `Released`, and `Concluded`.
- Added lease heartbeat handling and multi-source Fact support for Intent hyperedges.
- Added tests in `internal/blackboard/store_test.go` for fact append snapshots, claim/heartbeat/release, conclude with result Fact, and multi-fact source preservation.
- Added immutable Event contracts and event type constants in `internal/eventlog/event.go`.
- Added SQLite WAL event store in `internal/eventlog/sqlite.go`.
- Added migrations in `internal/eventlog/migrations.go`.
- Added Event Log tests in `internal/eventlog/sqlite_test.go` for append order, replay by run id, JSON payload round trip, WAL mode, declared event types, invalid events, and missing-run replay.
- Added Evidence Graph node and edge contracts in `internal/evidence/types.go`.
- Added Blackboard-to-Evidence projection in `internal/evidence/projector.go`.
- Added Evidence Graph tests in `internal/evidence/projector_test.go` for Fact-to-node projection, Intent-to-edge projection, concluded Intent result projection, and snapshot collection behavior.
- Added content-addressed Artifact Vault in `internal/artifact/vault.go`.
- Added Artifact Vault tests in `internal/artifact/vault_test.go` for write/read round trip, duplicate content deduplication, and invalid digest rejection.

## Not Implemented
- Event Log replay to rebuild the full projected state is deferred to the Replay milestone.

## Validation
- `GOPROXY=https://goproxy.cn,direct go test ./internal/goal -v`
- `GOPROXY=https://goproxy.cn,direct go test ./internal/blackboard -v`
- `GOPROXY=https://goproxy.cn,direct go test ./internal/eventlog -v`
- `GOPROXY=https://goproxy.cn,direct go test ./internal/evidence -v`
- `GOPROXY=https://goproxy.cn,direct go test ./internal/artifact -v`
- `GOPROXY=https://goproxy.cn,direct go test ./...`

## Next Step
- Start Milestone 2 with Task 2.1: Sisyphus Task Model.

## Risks / Decisions
- Goal weights currently use a small floating-point tolerance for normalized totals.
- Validation is intentionally limited to core contract correctness; scheduling and scoring logic belong to later Sisyphus and Frontier milestones.
- The current blackboard store is in-memory by design for Task 1.2; durable event capture belongs to Task 1.3.
- Store access returns snapshot copies so callers cannot mutate Facts or Intent source edges outside the store.
- SQLite uses `github.com/mattn/go-sqlite3`, which requires CGO; this environment has `CGO_ENABLED=1`.
- Go dependencies are fetched with `GOPROXY=https://goproxy.cn,direct` because `proxy.golang.org` is not reachable here.
- Evidence projection currently consumes Blackboard state directly; Event Log replay integration will be expanded in the Replay milestone.
