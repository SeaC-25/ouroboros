# Milestone 246: Fixture Evidence Status Visibility

## Summary

Hardened the read-only `completion-status` output so fixture-only completion evidence is visible before a final acceptance attempt.

When the completion audit result, manifest, and digest sidecar are otherwise ready but the linked evidence carries fixture markers, `completion-status` now reports:

```text
final_review_status=fixture_evidence_not_acceptable
fixture_evidence_detected=true
remaining_manual_step=replace_fixture_evidence_with_real_operator_evidence
final_acceptance_status=not_applicable
claim_100_ready=false
```

`completion-accept` still rejects fixture evidence with `fixture evidence cannot be final accepted`.

## Boundary

This is a visibility and final-review safety improvement. It does not make fixture evidence acceptable, does not claim 100%, does not write evidence from `completion-status`, and does not replace real operator-owned production evidence or final operator review.

Fixture detection now recognizes explicit fixture markers only. Bad or non-canonical linked evidence paths still flow to the existing path/digest validation errors instead of being collapsed into a fixture classification.

## Validation

RED:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test ./cmd/ouro -run TestCompletionStatusCommandSurfacesFixtureEvidence -count=1 -v
```

failed because fixture evidence still reported `final_review_status=operator_review_required`.

GREEN:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test ./cmd/ouro -run TestCompletionStatusCommandSurfacesFixtureEvidence -count=1 -v
```

passed after adding fixture status visibility.

Related completion regression:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test ./cmd/ouro -run 'TestCompletion(StatusCommand|AcceptCommand)' -count=1 -v
```

passed after fixing fixture detection so non-canonical linked evidence paths still report the original path validation errors.

Script regression:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test . -run 'TestFinalEvidenceFixtureSmokeScriptExercisesProductionAcceptanceAndCompletionAudit|TestReleaseMakefileAndCIEntrypoints' -count=1 -v
```

passed.

Built-binary smoke:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build make build
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build make smoke-final-evidence-fixture
```

passed, and the smoke output included `final_review_status=fixture_evidence_not_acceptable`, `fixture_evidence_detected=true`, `claim_100_ready=false`, `completion-accept fixture-rejected`, and `final-evidence-fixture: ok`.
