# Ouroboros TUI Studio Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Ouroboros as a TUI-first automated pentest compiler/IDE with blackboard state, multi-agent orchestration, native toolchain integration, model/runtime telemetry, and Web/API, Infra/Internal, CTF/Lab domain packs.

**Architecture:** Implement a Go core with explicit state contracts first, then layer TUI, agent swarm, toolchain runtime, strategy patterns, and domain packs on top. Agents never mutate shared state directly; the Swarm Orchestrator validates structured outputs and writes to the Blackboard/Evidence Graph. The product path must target real workspaces, real toolchain manifests, real provider adapters, and real domain-pack execution from the beginning. Test doubles are allowed only inside automated tests and fixtures.

**Tech Stack:** Go 1.23+, Cobra CLI, Bubble Tea/Lip Gloss/Bubbles TUI, SQLite WAL, YAML schema validation, filesystem artifact vault, native tool bundle manifests, provider adapters for model telemetry.

---

## Scope And Decomposition

This is a platform program, not a single feature. Implement it as independently testable slices:

1. **Foundation:** repository scaffold, config, logging, event log.
2. **State Core:** Goal Stack, Blackboard Protocol, Evidence Graph, Artifact Vault.
3. **Execution Brain:** Sisyphus Engine, Breadth/Depth Controller, Strategy Pattern Library.
4. **Agent Runtime:** Swarm Orchestrator, agent driver contracts, provider-backed agents, model usage ledger.
5. **Tool Runtime:** Native Toolchain Runtime, corpus tools, Tool Compiler.
6. **TUI Studio:** workspace, conversation/workbench, graph panels, runtime panels.
7. **Domain Packs:** Web/API, Infra/Internal, CTF/Lab minimal complete loops.
8. **Replay And Reports:** timeline lanes, replay, report export.

## Execution Policy

- Do not build a demo or toy product path. The deliverable is a fully usable TUI-first pentest compiler.
- Mock providers, fake runners, and fixtures may exist only for `go test ./...`; production commands must use real workspace configuration, real toolchain manifests, and real provider adapters.
- Every production feature must write durable events, artifacts, or state projections that can be replayed, reported, and inspected in the TUI.
- Each milestone must leave the repository closer to a usable compiler, not only to a passing isolated unit test.

## Progress Documentation Requirement

After each milestone or implementation phase, create or update a progress document under:

```text
docs/progress/milestone-XX-<slug>.md
```

Each progress document must include:

- **Implemented:** concrete modules, commands, files, and behaviors completed.
- **Not Implemented:** planned capabilities still missing or intentionally deferred.
- **Validation:** commands run, test results, and any manual checks.
- **Next Step:** the exact next milestone/task to implement.
- **Risks / Decisions:** blockers, tradeoffs, architecture decisions, and unresolved questions.

Do not mark a milestone complete until its progress document is written.

## Target Repository Layout

```text
cmd/ouro/main.go
internal/app/app.go
internal/config/
internal/eventlog/
internal/artifact/
internal/blackboard/
internal/evidence/
internal/goal/
internal/sisyphus/
internal/frontier/
internal/patterns/
internal/swarm/
internal/modelruntime/
internal/toolchain/
internal/compiler/
internal/tui/
internal/domain/webapi/
internal/domain/infra/
internal/domain/ctf/
configs/
toolchains/
test/fixtures/
```

## Milestone 0: Project Foundation

### Task 0.1: Go Module And CLI Shell

**Files:**
- Create: `go.mod`
- Create: `cmd/ouro/main.go`
- Create: `internal/app/app.go`
- Test: `internal/app/app_test.go`

- [x] **Step 1: Initialize module**

Run:

```bash
go mod init github.com/ouroboros/ouroboros
go get github.com/spf13/cobra@latest
```

Expected: `go.mod` exists and includes Cobra.

- [x] **Step 2: Add app metadata**

Implement `internal/app/app.go` with:

```go
package app

const Name = "ouro"
const Version = "0.1.0-dev"

func Banner() string {
	return Name + " " + Version
}
```

- [x] **Step 3: Add CLI entrypoint**

Implement `cmd/ouro/main.go` with `root`, `init`, `run`, `replay`, `toolchain`, and `doctor` commands. `run` may print `not implemented` in this task; commands must parse.

- [x] **Step 4: Add metadata test**

Test:

```go
func TestBanner(t *testing.T) {
	if app.Banner() != "ouro 0.1.0-dev" {
		t.Fatalf("unexpected banner: %q", app.Banner())
	}
}
```

Run:

```bash
go test ./...
```

Expected: PASS.

## Milestone 1: State Core

### Task 1.1: Goal Stack Contracts

**Files:**
- Create: `internal/goal/types.go`
- Create: `internal/goal/validate.go`
- Test: `internal/goal/goal_test.go`

- [x] Define `GoalStack` with `Mission`, `Coverage`, `Evidence`, `Report`, `Weights`, and `SuccessCriteria`.
- [x] Validate non-empty mission and normalized weights.
- [x] Add tests for valid stack, empty mission, and invalid weights.

Acceptance:

```bash
go test ./internal/goal -v
```

Expected: all tests pass.

### Task 1.2: Blackboard Protocol

**Files:**
- Create: `internal/blackboard/types.go`
- Create: `internal/blackboard/store.go`
- Test: `internal/blackboard/store_test.go`

- [x] Define `Fact`, `Intent`, `Hint`, `Lease`, and `ProjectState`.
- [x] Facts are append-only.
- [x] Intents support `Open`, `Claimed`, `Concluded`, `Released`.
- [x] Implement in-memory store first.
- [x] Add tests for fact append, intent claim heartbeat release, conclude, and multi-fact source intent.

Acceptance:

```bash
go test ./internal/blackboard -v
```

Expected: all tests pass.

### Task 1.3: Event Log And SQLite Store

**Files:**
- Create: `internal/eventlog/event.go`
- Create: `internal/eventlog/sqlite.go`
- Create: `internal/eventlog/migrations.go`
- Test: `internal/eventlog/sqlite_test.go`

- [x] Define immutable `Event`.
- [x] Persist events to SQLite WAL.
- [x] Add event type constants for goal, fact, intent, hint, attempt, agent, tool, parser, verification, mutation, replay.
- [x] Add tests for append order, replay by run id, and JSON payload round trip.

Acceptance:

```bash
go test ./internal/eventlog -v
```

Expected: all tests pass.

### Task 1.4: Evidence Graph Projection

**Files:**
- Create: `internal/evidence/types.go`
- Create: `internal/evidence/projector.go`
- Test: `internal/evidence/projector_test.go`

- [x] Define nodes: `Target`, `Surface`, `Identity`, `Artifact`, `Finding`, `Attempt`, `Hypothesis`, `Agent`.
- [x] Define edges: `observed_by`, `supports`, `contradicts`, `derived_from`, `verified_by`, `requires`, `unlocks`, `exhausted_by`, `assigned_to`, `produced_by`.
- [x] Project graph state from events and blackboard objects.
- [x] Add tests for fact-to-node projection and intent-to-edge projection.

### Task 1.5: Artifact Vault

**Files:**
- Create: `internal/artifact/vault.go`
- Test: `internal/artifact/vault_test.go`

- [x] Implement content-addressed filesystem artifact storage.
- [x] Store artifacts by SHA-256 digest with size, path, and media type metadata.
- [x] Add read-back and deduplication behavior.
- [x] Add tests for write/read round trip, duplicate content, and invalid digest rejection.

## Milestone 2: Sisyphus Engine And Frontier

### Task 2.1: Sisyphus Task Model

**Files:**
- Create: `internal/sisyphus/types.go`
- Create: `internal/sisyphus/engine.go`
- Test: `internal/sisyphus/engine_test.go`

- [x] Define task kinds: `bootstrap`, `reason`, `explore`, `conclude`.
- [x] Define `Attempt`, `FailureReason`, `MutationHint`, and `TaskResult`.
- [x] Implement one deterministic engine step over an in-memory blackboard.
- [x] Add tests: new project schedules bootstrap; open intent schedules explore; no open intent schedules reason.

### Task 2.2: Conclude Fallback

**Files:**
- Modify: `internal/sisyphus/engine.go`
- Test: `internal/sisyphus/conclude_test.go`

- [x] Add fallback path when attempt times out, output parsing fails, or worker exits with partial transcript.
- [x] Fallback returns fact summary, failure reason, and reusable leads only.
- [x] Add tests that failed attempt still emits a `conclude` task and writes a useful Fact.

### Task 2.3: Breadth / Depth Controller

**Files:**
- Create: `internal/frontier/types.go`
- Create: `internal/frontier/scoring.go`
- Create: `internal/frontier/queue.go`
- Test: `internal/frontier/frontier_test.go`

- [x] Define `CandidatePath` with target, hypothesis, expected unlock, cost, confidence, novelty, duplicate risk, noise, exhausted penalty.
- [x] Implement score formula from the design doc.
- [x] Implement priority queue ordering.
- [x] Add tests for high mission value, exhausted penalty, duplicate penalty, and novelty.

### Task 2.4: Coverage Matrix

**Files:**
- Create: `internal/frontier/coverage.go`
- Test: `internal/frontier/coverage_test.go`

- [x] Define coverage dimensions for Web/API, Infra/Internal, and CTF/Lab.
- [x] Track discovered/tested/verified/exhausted counters.
- [x] Add tests for update and heatmap status buckets.

## Milestone 3: Strategy Pattern Library

### Task 3.1: Pattern Schema

**Files:**
- Create: `internal/patterns/types.go`
- Create: `internal/patterns/loader.go`
- Create: `configs/patterns/ralphloop.yaml`
- Create: `configs/patterns/explore_plan_execute.yaml`
- Create: `configs/patterns/ant_colony.yaml`
- Test: `internal/patterns/loader_test.go`

- [x] Define graph pattern schema with nodes, edges, conditions, max steps, metadata.
- [x] Load YAML pattern files.
- [x] Validate node references and terminal result nodes.
- [x] Add tests for the three built-in patterns.

### Task 3.2: Pattern Evaluator

**Files:**
- Create: `internal/patterns/evaluator.go`
- Test: `internal/patterns/evaluator_test.go`

- [x] Implement deterministic transition evaluation using named condition results.
- [x] Add tests for RalphLoop retry, EPE progression, and Ant Colony deposit.

## Milestone 4: Agent Swarm Runtime

### Task 4.1: Agent Contracts

**Files:**
- Create: `internal/swarm/types.go`
- Create: `internal/swarm/output_contracts.go`
- Test: `internal/swarm/contracts_test.go`

- [x] Define `AgentRole`, `AgentState`, `AgentTask`, `AgentResult`, `AgentThread`, and `StructuredOutput`.
- [x] Roles include planner, web-api, infra, ctf, verifier, evidence, reporter, context, graph.
- [x] Validate that agent output can only propose facts, intents, hints, findings, or mutations.
- [x] Add tests for valid output and rejected malformed output.

### Task 4.2: Swarm Orchestrator

**Files:**
- Create: `internal/swarm/orchestrator.go`
- Create: `internal/swarm/driver.go`
- Test: `internal/swarm/orchestrator_test.go`

- [x] Implement orchestrator as sole writer to blackboard/evidence state.
- [x] Define the agent driver interface for real provider-backed workers.
- [x] Use deterministic test drivers only in test files.
- [x] Add tests that agents cannot directly mutate state.
- [x] Add tests for progress aggregation and blocked state.

### Task 4.3: Agent Timeline Lanes

**Files:**
- Create: `internal/swarm/timeline.go`
- Test: `internal/swarm/timeline_test.go`

- [x] Define lanes: mission, agent, observer, blackboard, toolchain, submission-report.
- [x] Convert event log entries into timeline events.
- [x] Add tests for lane assignment and chronological ordering.

## Milestone 5: Model Runtime Monitor

### Task 5.1: Usage Ledger

**Files:**
- Create: `internal/modelruntime/types.go`
- Create: `internal/modelruntime/ledger.go`
- Test: `internal/modelruntime/ledger_test.go`

- [x] Track provider, model, role, input tokens, output tokens, cache read/write, cost, latency, failure count.
- [x] Aggregate by agent, role, run, and model.
- [x] Add tests for aggregation and cost calculation.

### Task 5.2: Context Health

**Files:**
- Create: `internal/modelruntime/context_health.go`
- Test: `internal/modelruntime/context_health_test.go`

- [x] Track used tokens, max window, pinned objects, next compaction threshold.
- [x] Implement thresholds: 70 rolling summary, 85 compact attempts, 95 checkpoint.
- [x] Add tests for each threshold state.

## Milestone 6: Native Toolchain Runtime

### Task 6.1: Toolchain Manifest

**Files:**
- Create: `internal/toolchain/manifest.go`
- Create: `toolchains/web-api-pack/manifest.yaml`
- Create: `toolchains/infra-pack/manifest.yaml`
- Create: `toolchains/ctf-pack/manifest.yaml`
- Test: `internal/toolchain/manifest_test.go`

- [x] Define tool capability manifest with name, version, capabilities, runner, parser, healthcheck.
- [x] Load manifests from `toolchains/*/manifest.yaml`.
- [x] Add tests for manifest parsing, missing capability, and version lock.

### Task 6.2: Toolchain Health

**Files:**
- Create: `internal/toolchain/health.go`
- Test: `internal/toolchain/health_test.go`

- [x] Implement health states: ready, missing, failed, update_available.
- [x] Use mock runner for tests.
- [x] Add tests for all states.

### Task 6.3: Native Corpus Tools

**Files:**
- Create: `internal/toolchain/corpus.go`
- Create: `test/fixtures/corpus/nuclei/http/test-template.yaml`
- Test: `internal/toolchain/corpus_test.go`

- [x] Implement corpus search/read over bundled directories.
- [x] Return artifact refs and snippets.
- [x] Add tests for search by keyword, path traversal rejection, and bounded results.

## Milestone 7: Tool Compiler

### Task 7.1: Compiler Contracts

**Files:**
- Create: `internal/compiler/types.go`
- Create: `internal/compiler/compiler.go`
- Test: `internal/compiler/compiler_test.go`

- [x] Define semantic actions: discover_assets, fingerprint_surface, enumerate_paths, generate_candidates, verify_candidate, collect_evidence, produce_report.
- [x] Compile semantic action plus evidence graph slice into `PlanGraph`.
- [x] Add tests that compiler chooses capabilities, not hard-coded tool names.

### Task 7.2: Parser Binding

**Files:**
- Create: `internal/compiler/parser_binding.go`
- Test: `internal/compiler/parser_binding_test.go`

- [x] Bind tool output to parser specs.
- [x] Convert parsed output into blackboard/evidence events.
- [x] Add tests for successful parse and parse failure triggering conclude fallback.

## Milestone 8: TUI Studio

### Task 8.1: TUI Shell

**Files:**
- Create: `internal/tui/app.go`
- Create: `internal/tui/layout.go`
- Create: `internal/tui/statusbar.go`
- Test: `internal/tui/layout_test.go`

- [x] Create Bubble Tea app shell.
- [x] Implement three-column layout and bottom status bar.
- [x] Add viewport resize tests for minimum widths.

### Task 8.2: Workspace Panel

**Files:**
- Create: `internal/tui/workspace.go`
- Test: `internal/tui/workspace_test.go`

- [x] Render mission.yaml, scope.yaml, artifacts, evidence, attempts, findings, reports, replay, toolchain.
- [x] Add tests for sorted tree display and selected file state.

### Task 8.3: Mission Conversation And Workbench

**Files:**
- Create: `internal/tui/conversation.go`
- Create: `internal/tui/workbench.go`
- Test: `internal/tui/workbench_test.go`

- [x] Display mission events, Sisyphus round, current plan, attempt result, failure reason, mutation.
- [x] Switch center panel to selected Agent Workbench.
- [x] Add tests for default conversation and agent-focused workbench.

### Task 8.4: Graph Stack

**Files:**
- Create: `internal/tui/attack_graph.go`
- Create: `internal/tui/agent_graph.go`
- Create: `internal/tui/inspector.go`
- Test: `internal/tui/graph_stack_test.go`

- [x] Render text graph for Attack Graph.
- [x] Render Agent Graph with status and progress.
- [x] Inspector shows selected node details.
- [x] Add tests for focus and node selection.

### Task 8.5: Runtime Tabs

**Files:**
- Create: `internal/tui/runtime_tabs.go`
- Test: `internal/tui/runtime_tabs_test.go`

- [x] Implement tabs: Node Inspector, Model Runtime, Agent Runtime, Toolchain Health, Coverage Matrix, Frontier Queue, Timeline Lanes.
- [x] Add tests for tab switching.

## Milestone 9: Domain Packs

### Task 9.1: Domain Pack SDK

**Files:**
- Create: `internal/domain/sdk.go`
- Test: `internal/domain/sdk_test.go`

- [x] Define domain pack interface: capabilities, hypotheses, coverage dimensions, parsers, strategy patterns.
- [x] Add test fake pack.

### Task 9.2: Web/API Pack Minimal Loop

**Files:**
- Create: `internal/domain/webapi/pack.go`
- Create: `internal/domain/webapi/hypotheses.go`
- Test: `internal/domain/webapi/pack_test.go`

- [x] Provide capabilities for site discovery, route enumeration, auth-state tracking, candidate generation, evidence collection.
- [x] Add tests that a URL target produces initial hypotheses and coverage dimensions.

### Task 9.3: Infra/Internal Pack Minimal Loop

**Files:**
- Create: `internal/domain/infra/pack.go`
- Create: `internal/domain/infra/hypotheses.go`
- Test: `internal/domain/infra/pack_test.go`

- [x] Provide capabilities for host discovery, service enumeration, identity graph, path hypothesis, evidence collection.
- [x] Add tests that CIDR target produces host/service coverage.

### Task 9.4: CTF/Lab Pack Minimal Loop

**Files:**
- Create: `internal/domain/ctf/pack.go`
- Create: `internal/domain/ctf/hypotheses.go`
- Test: `internal/domain/ctf/pack_test.go`

- [x] Provide capabilities for challenge classification, artifact analysis, fast hypothesis generation, candidate verification, result proof.
- [x] Add tests that challenge target produces three initial hypotheses before broad enumeration.

## Milestone 10: Replay And Reports

### Task 10.1: Replay Store

**Files:**
- Create: `internal/eventlog/replay.go`
- Create: `internal/tui/replay.go`
- Test: `internal/eventlog/replay_test.go`

- [x] Rebuild blackboard, evidence graph, timeline lanes, model usage, and TUI status from event log.
- [x] Add tests with fixture event log.

### Task 10.2: Report Projection

**Files:**
- Create: `internal/report/report.go`
- Create: `internal/report/markdown.go`
- Test: `internal/report/report_test.go`

- [x] Project verified findings, paths, evidence refs, exhausted areas, and next-run suggestions into Markdown.
- [x] Add tests for report sections from fixture evidence graph.

## Milestone 11: Production Core Bring-Up

### Task 11.1: Real Workspace Run Path

**Files:**
- Create: `internal/app/run.go`
- Create: `internal/app/workspace.go`
- Create: `internal/app/run_test.go`
- Create: `configs/providers.example.yaml`

- [x] Implement `ouro init <workspace>` to create a real workspace with mission, scope, artifacts, evidence, attempts, findings, reports, replay, and toolchain directories.
- [x] Implement `ouro run <workspace>` using configured provider adapters, domain packs, toolchain manifests, event log, blackboard, evidence projection, TUI state projection, and report generation.
- [x] Reject runs when provider config, scope, storage, or required toolchain manifests are invalid.
- [x] Keep test doubles inside tests to verify run orchestration without network or live targets.

Acceptance:

```bash
go test ./...
go run ./cmd/ouro init ./workspace
go run ./cmd/ouro doctor ./workspace
go run ./cmd/ouro run ./workspace
```

Expected: tests pass; `init` creates the workspace; `doctor` validates configuration and toolchain status; `run` starts the real production execution path and records durable events.

## Milestone 12: Build And Distribution

### Task 12.1: Build Script

**Files:**
- Create: `Makefile`
- Create: `scripts/build.sh`

- [x] `make test` runs `go test ./...`.
- [x] `make build` builds `bin/ouro`.
- [x] `make doctor` runs `go run ./cmd/ouro doctor`.

### Task 12.2: Developer Docs

**Files:**
- Create: `docs/development.md`
- Modify: `AGENTS.md`

- [x] Document repository layout, build/test commands, and where to add new domain packs, tool manifests, strategy patterns, and TUI panels.
- [x] Update `AGENTS.md` after implementation structure exists.

## Cross-Cutting Acceptance Criteria

- `go test ./...` passes.
- `ouro doctor` reports config, storage, toolchain manifests, and model provider config status.
- `ouro init ./workspace` creates a workspace with mission, scope, artifacts, evidence, attempts, findings, reports, replay, and toolchain dirs.
- `ouro run ./workspace` starts the real production execution path using configured providers, domain packs, and native toolchain manifests.
- TUI status bar displays model, context, agents, breadth, depth, tools, and cost.
- Agent Graph and Attack Graph can be focused independently.
- Replay reconstructs timeline lanes from event log.
- Users do not need to manually install bottom-layer tools for bundled toolchain-supported runs.
- Every completed milestone has a progress document under `docs/progress/` covering implemented work, missing work, validation, next step, and risks/decisions.

## Plan Self-Review

- Spec coverage: TUI Studio, Blackboard Protocol, Sisyphus Engine, Breadth/Depth, Agent Swarm, Model Runtime, Native Toolchain, Tool Compiler, Domain Packs, Replay, and Reports each have implementation milestones.
- Placeholder scan: no unresolved placeholder markers, no vague future-work notes, and no open-ended "add tests" without concrete target.
- Type consistency: shared names are consistent with the architecture document: Goal Stack, Blackboard Protocol, Evidence Graph, Swarm Orchestrator, Strategy Pattern Library, Native Toolchain Runtime.
