# Milestone 10: Replay And Reports

## Implemented
- Added Replay Store projection in `internal/eventlog/replay.go`.
- Added replay snapshot contracts for Blackboard state, Evidence Graph, timeline lanes, model usage aggregate, agent state, and TUI status fields.
- Added deterministic replay sorting by timestamp and event ID.
- Added mixed-run rejection so a replay snapshot cannot silently combine unrelated runs.
- Added TUI replay projection in `internal/tui/replay.go`.
- TUI replay projection updates status bar fields, Attack Graph nodes/edges, Agent Graph nodes, Model Runtime tab, Agent Runtime tab, Toolchain Health tab, and Timeline Lanes tab rows.
- Added fixture-driven Replay Store tests in `internal/eventlog/replay_test.go`.
- Added TUI replay projection tests in `internal/tui/replay_test.go`.
- Added Report Projection in `internal/report/report.go`.
- Added Markdown renderer in `internal/report/markdown.go`.
- Report Projection now derives verified findings, attack paths, evidence refs, exhausted areas, and next-run suggestions from an Evidence Graph.
- Added fixture-driven report tests in `internal/report/report_test.go`.

## Not Implemented
- Replay Store is not yet wired into the production `ouro run` path.
- Replay persistence still depends on the existing SQLite event log; this task adds projection from loaded events, not a new storage backend.
- Report generation is not yet wired into the production `ouro run` path.

## Validation
- `GOPROXY=https://goproxy.cn,direct go test ./internal/eventlog -run Replay -v`
- `GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run Replay -v`
- `GOPROXY=https://goproxy.cn,direct go test ./internal/eventlog -v`
- `GOPROXY=https://goproxy.cn,direct go test ./internal/tui -v`
- `GOPROXY=https://goproxy.cn,direct go test ./internal/report -v`
- `GOPROXY=https://goproxy.cn,direct go test ./...`

## Next Step
- Continue Milestone 11 with Task 11.1: Real Workspace Run Path.

## Risks / Decisions
- `internal/eventlog` does not import `internal/swarm` or `internal/tui`; replay lanes are defined locally to avoid package cycles.
- Replay payloads use the existing event payload convention from parser binding and runtime telemetry rather than a new event table schema.
- TUI status is derived from the replay snapshot, with model/cost from model usage events, agent count from agent events, and tool health from tool events.
- Report Projection treats `finding` nodes as verified findings, `artifact` nodes as evidence refs, `exhausted` source nodes as exhausted areas, and `suggestion` source nodes as next-run suggestions.
- Attack paths are reconstructed from Evidence Graph `derived_from` and `requires` edges.
