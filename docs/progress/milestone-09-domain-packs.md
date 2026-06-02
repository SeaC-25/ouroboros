# Milestone 09: Domain Packs

## Implemented
- Added Domain Pack SDK in `internal/domain/sdk.go`.
- Added target kinds for URL, CIDR, challenge, and artifact inputs.
- Added `Capability`, `Hypothesis`, and `Pack` contracts.
- Added pack validation for name, domain, capabilities, coverage dimensions, parser specs, and strategy patterns.
- Added tests in `internal/domain/sdk_test.go` with a fake pack covering capabilities, initial hypotheses, coverage dimensions, parsers, strategy patterns, and incomplete pack validation.
- Added Web/API Pack in `internal/domain/webapi/pack.go`.
- Added URL-target initial hypotheses in `internal/domain/webapi/hypotheses.go`.
- Added Web/API capabilities for site discovery, route enumeration, auth-state tracking, candidate generation, and evidence collection.
- Added Web/API coverage dimensions for routes, params, and auth states.
- Added parser specs and strategy patterns for Web/API execution.
- Added tests in `internal/domain/webapi/pack_test.go` for capability declarations, URL hypotheses, coverage dimensions, and non-URL target rejection.
- Added Infra/Internal Pack in `internal/domain/infra/pack.go`.
- Added CIDR-target initial hypotheses in `internal/domain/infra/hypotheses.go`.
- Added Infra/Internal capabilities for host discovery, service enumeration, identity graph, path hypothesis generation, and evidence collection.
- Added Infra/Internal coverage dimensions for hosts, services, and identities.
- Added parser specs and strategy patterns for Infra/Internal execution.
- Added tests in `internal/domain/infra/pack_test.go` for capability declarations, CIDR hypotheses, host/service/identity coverage, and non-CIDR target rejection.
- Added CTF/Lab Pack in `internal/domain/ctf/pack.go`.
- Added challenge-target initial hypotheses in `internal/domain/ctf/hypotheses.go`.
- Added CTF/Lab capabilities for challenge classification, artifact analysis, fast hypothesis generation, candidate verification, and result proof.
- Added CTF/Lab coverage dimensions for hypotheses, artifacts, and proofs.
- Added parser specs and strategy patterns for CTF/Lab execution.
- Added tests in `internal/domain/ctf/pack_test.go` for capability declarations, three hypothesis-first challenge hypotheses, and non-challenge target rejection.

## Not Implemented
- Domain packs are not yet wired into Tool Compiler selection or production run orchestration.

## Validation
- `GOPROXY=https://goproxy.cn,direct go test ./internal/domain -v`
- `GOPROXY=https://goproxy.cn,direct go test ./internal/domain/... -v`
- `GOPROXY=https://goproxy.cn,direct go test ./...`

## Next Step
- Continue Milestone 10 with Task 10.1: Replay Log.

## Risks / Decisions
- Domain packs expose semantic actions and expected evidence, not concrete tool names.
- `InitialHypotheses` accepts a typed target and can return an error so each pack can reject incompatible targets.
- Validation requires strategy patterns and parser specs so future domain packs cannot silently bypass compiler/parser integration.
- Web/API initial hypotheses deliberately cover breadth first: discovery, route enumeration, auth-state mapping, and candidate generation.
- Infra/Internal initial hypotheses require parseable CIDR targets and start with hosts, services, identities, then internal path candidates.
- CTF/Lab initial hypotheses deliberately avoid broad enumeration: classify the challenge, analyze artifacts, then generate fast solve hypotheses.
