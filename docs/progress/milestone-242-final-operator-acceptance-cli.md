# Milestone 242: Final Operator Acceptance CLI

## Summary

Added `ouro completion-accept <audit-log-dir> --reviewed-by <operator> --accept-real-evidence` as the explicit final operator acceptance path after completion evidence reaches `ready-for-final-review`.

The command writes `completion-final-acceptance.txt` only after the existing completion result, evidence manifest, and manifest digest sidecar are ready for final review. The acceptance artifact records final operator identity/time, binds the current result, manifest, digest sidecar, production summary, completion attestation, and deployment policy by absolute path and SHA-256, and records:

- `real_operator_evidence_reviewed=true`
- `fixture_evidence_accepted=false`
- `claim_100_ready=true`

`ouro completion-status <audit-log-dir>` now reads the optional final acceptance artifact. It keeps the pre-acceptance state at `ready-for-final-review` with `claim_100_ready=false`, and reports `claim_100_ready=true` only when the acceptance artifact is schema-valid, path-bound, digest-bound to the current evidence files, and not fixture-marked.

## Boundary

The final acceptance file is local mutable review evidence. It is not a cryptographic signature, append-only storage, remote attestation, or replacement for real operator evidence review.

Fixture evidence remains fixture-only. `scripts/smoke_final_evidence_fixture.sh` now verifies that `completion-accept` rejects fixture-marked evidence and cannot produce `claim_100_ready=true`.

A sidecar review found two acceptance hardening gaps before this milestone was closed: linked evidence digest drift and attestation-only fixture markers. The final implementation rejects production summary, attestation, or policy bytes that no longer match the manifest-recorded SHA-256 values, reads the attestation body for fixture markers, requires an absolute audit log directory before writing acceptance evidence, and revalidates opened evidence files with Lstat/Open/Stat/SameFile before parsing or hashing.

## Validation

RED:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test ./cmd/ouro -run 'Test(RootCommandIncludesProductionCommands|CompletionAcceptCommand|CompletionStatusCommand)' -count=1 -v
```

failed before implementation because `completion-accept` did not exist and `completion-status` ignored final acceptance evidence.

RED:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test . -run TestFinalEvidenceRunbookDocumentsRequiredFields -count=1 -v
```

failed while synchronizing docs because `docs/development.md` lacked `completion-final-acceptance.txt`.

GREEN:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test . -run TestFinalEvidenceRunbookDocumentsRequiredFields -count=1 -v
```

passed after the runbook documented `completion-accept`, `completion-final-acceptance.txt`, `real_operator_evidence_reviewed=true`, `fixture_evidence_accepted=false`, and the real-evidence-only `claim_100_ready=true` path.

Security hardening RED:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test ./cmd/ouro -run 'TestCompletionAcceptCommandRejects(LinkedEvidenceDigestDrift|FixtureAttestationBody|RelativeAuditLogDir)' -count=1 -v
```

failed before the fix because `completion-accept` accepted linked evidence after manifest-recorded digest drift, accepted fixture markers present only in the attestation body, and wrote an acceptance artifact when the audit log directory was relative.

Focused final validation:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test ./cmd/ouro -run 'Test(RootCommandIncludesProductionCommands|CompletionAcceptCommand|CompletionStatusCommand)' -count=1 -v
```

passed.

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test . -run 'TestFinalEvidenceRunbookDocumentsRequiredFields|TestFinalEvidenceRunbookBashBlocksAreCopyPasteSafe|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck|TestDocumentationBashBlocksPassShellSyntaxCheck|TestReleaseMakefileAndCIEntrypoints|TestFinalEvidenceFixtureSmokeScriptExercisesProductionAcceptanceAndCompletionAudit' -count=1 -v
```

passed.

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test ./cmd/ouro -count=1
sh -n scripts/smoke_final_evidence_fixture.sh scripts/smoke_production_acceptance.sh scripts/audit_completion.sh
git diff --check
git diff --name-only -- '*.go' | xargs -r gofmt -l
```

passed clean.
