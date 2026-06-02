# Milestone 07: Tool Compiler

## Implemented
- Added Tool Compiler contract types in `internal/compiler/types.go`.
- Added semantic actions: `discover_assets`, `fingerprint_surface`, `enumerate_paths`, `generate_candidates`, `verify_candidate`, `collect_evidence`, and `produce_report`.
- Added `CompileRequest`, `EvidenceSlice`, `PlanGraph`, `PlanNode`, and `PlanEdge`.
- Added `internal/compiler/compiler.go` with capability-based manifest matching and deterministic plan node ordering.
- Added tests in `internal/compiler/compiler_test.go` for semantic action declarations, capability-based selection, evidence input propagation, expected evidence output, and missing capability rejection.
- Added Parser Binding in `internal/compiler/parser_binding.go`.
- Added JSON parser specs that map tool output fields into Blackboard Facts.
- Added parser and fact event projection through `eventlog.Event` values.
- Added parse failure handling that calls `sisyphus.Engine.ConcludeFallback` and preserves partial transcript leads.
- Added tests in `internal/compiler/parser_binding_test.go` for successful parser binding and parse failure fallback.
- Added optional request-level compile scoring inputs: `CompileBudget`, `DomainPriority`, `CandidateFrontierScore`, and `CandidateHistory`.
- `CompileRequest` can now rank candidate plan nodes by domain priority, `frontier.Score`, and historical success/failure signals, then cap the output with `Budget.MaxPlanNodes` while preserving deterministic capability/name tie-breaks.
- `PlanNode` now exposes `SelectionScore` and `SelectionSignals`, and `PlanGraph.Warnings` records budget truncation so later IDE/runtime views can explain why a candidate was selected or dropped.

## Not Implemented
- Production `RunWorkspace` and worker-placement compilation still use the default unscored compile path; feeding runtime budgets, frontier rows, and historical ledgers into `CompileRequest` is a later orchestration slice.
- Parser binding currently supports JSON object output only; richer parser adapters and streaming parser output are deferred.
- Parser/fact events are returned for the caller to persist; direct durable event store writes are deferred to the production run path.

## Validation
- `GOPROXY=https://goproxy.cn,direct go test ./internal/compiler -v`
- `GOPROXY=https://goproxy.cn,direct go test ./internal/compiler ./internal/app ./cmd/ouro -run 'Compile|RunWorkspace|RuntimeWorkerPlacement|WorkerPlacement|RunCommand' -v`
- `GOPROXY=https://goproxy.cn,direct go test ./...`

## Next Step
- Continue Milestone 8 with Task 8.1: TUI Shell.

## Risks / Decisions
- Compiler selection is intentionally capability-based. Manifest names are treated as bundle identifiers, not as a routing API for agents.
- `PlanGraph` currently compiles independent plan nodes for matching capabilities. Dependency edges should be added when parser binding and multi-step tool chains are introduced.
- Request-level scoring is optional and side-effect free: it only changes plan ordering/truncation for callers that provide signals, and it does not execute tools, contact targets, or alter safety bypass behavior.
- The built-in capability mapping is minimal and should move behind Domain Pack SDK priorities once production orchestration passes domain-pack priorities into `CompileRequest`.
- Parse failures are routed into Sisyphus conclude fallback instead of returning only parser errors, so partial transcripts can still become facts and mutation hints.
