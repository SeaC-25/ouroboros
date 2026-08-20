# Milestone 198: Production Acceptance IDE Child-Smoke Evidence

## Summary

Bound the direct AI chat IDE proof into final production evidence.

Milestone 197 made `ouro chat <workspace>` directly usable and added `chat --help` proof to `smoke_direct_use.sh`. This milestone makes that proof visible to the top-level production acceptance and final completion audit chain instead of leaving it inside only a nested local smoke log.

## Changes

- `scripts/smoke_production_acceptance.sh` now records:
  - `ide_direct_use_script`
  - `ide_direct_use_script_sha256`
  - `ide_ai_provider_script`
  - `ide_ai_provider_script_sha256`
  - `ide_direct_use_log`
  - `ide_direct_use_log_sha256`
  - `ide_ai_provider_log`
  - `ide_ai_provider_log_sha256`
- The IDE acceptance stage now verifies the child logs before accepting:
  - `ide-smoke-logs/direct-use.txt` contains `direct-use-smoke: ok`
  - `ide-smoke-logs/direct-use.txt` contains `chat: available`
  - `ide-smoke-logs/ai-provider.txt` contains `ai-provider-smoke: ok`
- `scripts/audit_completion.sh` now requires the IDE child scripts to match the maintained repo scripts and the child logs to be canonical production-summary siblings.
- Successful completion audit result and evidence manifest now expose the IDE child script/log evidence for final review.
- README, `docs/development.md`, `task_plan.md`, `findings.md`, and `progress.md` document the evidence binding.

## Safety Boundary

This remains evidence binding only. It does not execute model suggestions, call real providers, contact targets, dispatch workers, run managed payloads, or turn `chat` into an autonomous execution path.

The direct-use child proof is still a smoke of command availability and safe local IDE wiring. True production completion still requires real operator-owned evidence, an approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.

## Validation

- RED confirmed:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run TestCompletionAuditScriptRejectsIDEChildDirectUseLogWithoutChatEntrypointMarker -count=1 -v`
- Focused GREEN:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(RejectsIDEChildDirectUseLogWithoutChatEntrypointMarker|ValidatesProductionSummaryAndAttestation)' -count=1 -v`
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestCompletionAuditScript(ValidatesProductionSummaryAndAttestation|RejectsIDEChildDirectUseLogWithoutChatEntrypointMarker|RejectsStageLogWithoutOKSentinel|RequiresCanonicalIDEChildLogPaths)' -count=1 -v`
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run TestProductionAcceptanceSmokeScriptExercisesOperatorInventoryAndPolicy -count=1 -v`
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestFinalEvidenceRunbookDocuments(StageProvenanceArtifactBindings|RequiredFields)|TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v`
- Broad regression:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(ProductionAcceptanceSmoke|CompletionAuditScript|FinalEvidenceRunbook|DocumentationBashBlocks)' -count=1 -v`
- Full validation:
  - `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`

Final build, strict readiness, script syntax, formatting, and diff hygiene are recorded in `progress.md` for the working session that completed this milestone.
