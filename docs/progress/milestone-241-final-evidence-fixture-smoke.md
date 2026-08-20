# Milestone 241: Final Evidence Fixture Smoke

## Summary

Added `scripts/smoke_final_evidence_fixture.sh` and `make smoke-final-evidence-fixture` as a local fixture-only final evidence regression smoke.

The script creates temporary operator-style evidence, reuses the bundled release-managed `nuclei-templates` payload and manifest SHA-256 so production acceptance, managed-payload acceptance, release-managed acceptance, completion audit, and `completion-status` agree on the same rule-bundle digest, and prints `claim_100_ready=false`.

## Boundary

This smoke proves the final evidence mechanics are wired together. It is not production evidence, does not execute managed payloads, does not contact providers/workers/endpoints/targets, and does not replace real operator-owned payload evidence, approved attestation review, manifest/digest sidecar review, or final human/operator acceptance.

## Validation

RED:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test . -run 'Test(ReleaseMakefileAndCIEntrypoints|FinalEvidenceFixtureSmokeScriptExercisesProductionAcceptanceAndCompletionAudit)$' -count=1 -v
```

failed because `smoke-final-evidence-fixture` and `scripts/smoke_final_evidence_fixture.sh` did not exist.

GREEN:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test . -run 'Test(ReleaseMakefileAndCIEntrypoints|FinalEvidenceFixtureSmokeScriptExercisesProductionAcceptanceAndCompletionAudit)$' -count=1 -v
```

passed after the script and Makefile target were added. The fixture smoke executed production acceptance, completion audit, and completion-status, and preserved `claim_100_ready=false`.

Direct smoke:

```bash
TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make smoke-final-evidence-fixture
```

passed with `production-acceptance: status=accepted mode=operator-owned`, `completion-audit: status=ready-for-final-review`, `completion-status: ... claim_100_ready=false`, and `final-evidence-fixture: ok`.
