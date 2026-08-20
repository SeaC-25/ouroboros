# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Ouroboros is a TUI-first automated penetration-testing compiler/IDE written in Go (module `github.com/ouroboros/ouroboros`, Go 1.26.2). It turns scoped objectives into auditable plans, compiled toolchain actions, runtime worker state, replayable evidence, and operator-gated execution. It is not an MCP tool-router: the LLM never picks raw tools — domain packs emit hypotheses, the compiler turns semantic actions into tool plans, and state only advances on evidence events.

The git repo and Go module root is this directory (`Ouroboros/`), not its parent.

## Build, Test, Run

All commands assume the China Go proxy already wired into the Makefile. When running `go` directly, prepend `GOPROXY=https://goproxy.cn,direct`.

- `make test` — full suite (`go test ./...`). This is what CI runs.
- Single package: `GOPROXY=https://goproxy.cn,direct go test ./internal/app/`
- Single test: `GOPROXY=https://goproxy.cn,direct go test ./internal/app/ -run TestName`
- `make build` — builds `bin/ouro` via `scripts/build.sh`.
- `make doctor` / `make doctor-strict` — init a temp workspace and run readiness checks. `doctor-strict` adds `--require-safety-authorization` and is part of CI.
- `make release` — local release archive under `dist/` with `SHA256SUMS`.
- `make smoke-*` — end-to-end smoke scripts under `scripts/` (ide, workflow, ai-provider, release-managed, production-acceptance, etc.). Use the matching `smoke_*.sh` to reproduce a single one.

CI (`.github/workflows/ci.yml`) gates on, in order: `make test` → `make doctor-strict` → `make build` → `make release`. Keep all four green.

### Driving the CLI

```
go run ./cmd/ouro init <workspace>        # scaffold workspace layout
go run ./cmd/ouro doctor <workspace>      # validate readiness (add --require-safety-authorization)
go run ./cmd/ouro run <workspace>         # offline production run; writes events + Markdown report
go run ./cmd/ouro run <workspace> --tui   # deterministic TUI snapshot (non-interactive)
go run ./cmd/ouro run <workspace> --interactive   # Bubble Tea TUI
go run ./cmd/ouro replay <workspace> <run-id>     # replay a recorded run from the event log
go run ./cmd/ouro chat <workspace>        # workspace-bound AI chat IDE
go run ./cmd/ouro toolchain health <workspace>    # built-in toolchain readiness
```

`cmd/ouro/main.go` registers a large surface of report-projection and replay-workbench subcommands; read the `AddCommand` block there before assuming a command does not exist.

## Architecture

The pipeline is goal-driven and evidence-constrained. Conceptual flow (see `docs/Ouroboros_平台架构设计书.md` for the authoritative design, in Chinese):

```
Goal Stack → Sisyphus Engine → Frontier (breadth/depth) → Swarm Orchestrator
  → Domain Pack hypotheses → Tool Compiler → Native Toolchain Runtime
  → Parsers/Normalizers → Blackboard + Evidence Graph → Verifier → Reporter
```

Core packages under `internal/`:

- `app/` (largest, ~56 source files) — the orchestration hub. Owns workspace init/doctor (`workspace.go`), the production run pipeline (`run.go`, `RunWorkspaceWithOptions`), worker queue/lease/dispatch/lifecycle machinery (`worker_*.go`), provider config + generation (`provider_*.go`), report projection (`report_projection*.go`), replay workbench (`replay*.go`), and one `*_parser.go` per supported tool (nmap, nuclei, sqlmap, ffuf, httpx, katana, dalfox, …). Tool parsers live here, not in `compiler/`.
- `compiler/` — `Compiler.Compile(CompileRequest) → PlanGraph`. Translates a `SemanticAction` into a ranked, budgeted toolchain plan (`PlanNode`/`PlanEdge`) and binds parsers (`parser_binding.go`). This is the only place semantic actions become concrete tool invocations.
- `domain/` — the Domain Pack SDK (`sdk.go`: `Pack` interface, `Target`, `Capability`, `Hypothesis`) plus the three built-in packs `webapi/`, `infra/`, `ctf/`. A `Pack` exposes capabilities, initial hypotheses, coverage dimensions, parser specs, and strategy patterns.
- `blackboard/` — multi-agent coordination layer. `MemoryStore` holds Facts, Intents (with claim/lease/heartbeat lifecycle), and Hints. The orchestrator is the single writer; state advances only on evidence events, never on model self-report.
- `evidence/` — the fact layer. `Graph` of typed `Node`/`Edge` (target → service → finding → hypothesis). This is the source of truth a report is projected from.
- `frontier/` — breadth/depth controller. A scored `Queue` of `CandidatePath`s plus a `CoverageMatrix` (domain × dimension) to decide whether to widen or deepen.
- `sisyphus/` — the persistent attempt loop: hypothesis → plan → attempt → evidence → failure analysis → mutation. `ConcludeFallback` salvages timed-out/aborted attempts into reusable facts instead of discarding context.
- `swarm/` — agent contracts (`AgentRole`, `AgentTask`, `Proposal`), the `Orchestrator` (driven by a `Driver` over the blackboard), and timeline `Lane`s.
- `toolchain/` — native toolchain manifest loading, the built-in appsec tool catalog, authored-tool validation, health checks, and managed-bundle metadata.
- `eventlog/` — SQLite event store (`OpenSQLite`) and `BuildReplaySnapshot`, which projects an append-only `Event` stream into a `ReplaySnapshot` (blackboard state, agents, timeline, toolchain status). Runs are durable and fully replayable.
- `modelruntime/`, `patterns/`, `report/`, `goal/`, `contracts/`, `artifact/`, `tui/` — model telemetry; strategy patterns (`configs/patterns/*.yaml`); evidence-graph→Markdown projection; goal stack; built-in input validation (`contracts.go` wires packs + manifests + patterns together); artifact handling; Bubble Tea TUI.

### Workspace layout

`ouro init` scaffolds a workspace with required files `mission.yaml`, `scope.yaml`, `providers.yaml` and required dirs `artifacts/`, `evidence/`, `attempts/`, `findings/`, `reports/`, `replay/`, `runtime/`, `toolchain/`. A workspace must contain the three toolchain packs (`web-api-pack`, `infra-pack`, `ctf-pack`) to pass doctor. The event log lives at `replay/events.db`.

## Conventions

- Format Go with `gofmt`. Tests are standard `*_test.go` co-located with sources (~165 test files; tests are expected for meaningful changes).
- The run pipeline is structured as discrete, event-emitting stages (`workspace-bootstrap`, `toolchain-health`, `hypothesis-seeding`, `tool-execution`, `report-projection`). New runtime behavior should emit stage events through `appendRunEvent` so it stays replayable.
- Adding a Domain Pack: create `internal/domain/<name>/` with `pack.go`, `hypotheses.go`, `pack_test.go`; add `toolchains/<pack>/manifest.yaml`, strategy patterns under `configs/patterns/`, and TUI panels under `internal/tui/`. Register it in `internal/contracts/contracts.go` (`BuiltInDomainPacks`). See `docs/development.md` for the extension map.
- After completing a milestone, write/update `docs/progress/milestone-XX-<slug>.md` before marking it done.

## Safety boundary

This workspace is for authorized security work and runs offline by default. Do not run offensive tools against live targets from here; keep analysis static unless authorization and scope are documented. `--require-safety-authorization` makes doctor fail when a disabled safety boundary lacks matching authorization metadata. Do not commit secrets, extracted credentials, or scan artifacts.

## Treat as immutable

`model/` holds upstream reference archives (zip files) — never modify them. `.analysis/` is a regenerable scratch extraction of those archives for static review only; it is not canonical source. Run reference-project checks (e.g. `cd .analysis/src/<proj> && pytest`) only inside that extraction, never against first-party packages.
