# Milestone 243: Completion Acceptance Path-Component Hardening

## Summary

Hardened `ouro completion-status` and `ouro completion-accept` so final completion evidence cannot be inspected, compared, or accepted through symlink-parent or `symlink/..` path spellings.

The commands now require the completion audit log directory, completion evidence files, manifest-linked production summary, completion attestation, deployment policy, final-acceptance linked paths, and final acceptance output path to be canonical absolute paths with no `.` or `..` spellings and no existing symlink path components. This extends the earlier leaf-symlink checks to the full existing path prefix and prevents `filepath.Clean` / `filepath.Abs` from silently turning alternate evidence spellings into reviewed paths.

## Boundary

This does not make `completion-final-acceptance.txt` a signature or append-only record. It only keeps the local mutable final acceptance record tied to the path spelling and bytes the operator reviewed.

Fixture evidence is still rejected and still cannot produce `claim_100_ready=true`.

## Validation

RED:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test ./cmd/ouro -run 'TestCompletion(StatusCommandRejectsSymlinkAuditLogDirParent|AcceptCommandRejectsLinkedEvidenceSymlinkParent)' -count=1 -v
```

failed before the fix because `completion-status` accepted an audit log directory under a symlinked parent and `completion-accept` accepted a manifest-linked production summary through a symlinked parent.

Additional RED:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test ./cmd/ouro -run 'TestCompletion(StatusCommandRejectsSymlinkAuditLogDirParent|StatusCommandRejectsSymlinkDotDotAuditLogDir|StatusCommandRejectsFinalAcceptanceSymlinkDotDotPathBinding|AcceptCommandRejectsLinkedEvidenceSymlinkParent|AcceptCommandRejectsLinkedEvidenceSymlinkDotDotPath)' -count=1 -v
```

failed because `assertCompletionEvidencePathComponentsNoSymlink` cleaned candidates before walking components, `sameCompletionEvidencePath` compared `Abs`/`Clean` values, and one final-acceptance test used `filepath.Join` in a way that cleaned away the intended `linked/../logs` bypass before the code under test saw it.

GREEN:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test ./cmd/ouro -run 'TestCompletion(StatusCommandRejectsSymlinkAuditLogDirParent|StatusCommandRejectsSymlinkDotDotAuditLogDir|StatusCommandRejectsFinalAcceptanceSymlinkDotDotPathBinding|AcceptCommandRejectsLinkedEvidenceSymlinkParent|AcceptCommandRejectsLinkedEvidenceSymlinkDotDotPath)' -count=1 -v
```

passed after adding canonical-path rejection before component walks, making final-acceptance link comparison require exact canonical string equality, checking linked evidence paths before fixture classification, and preserving `linked/../logs/...` in the regression tests through string concatenation.

Focused completion CLI validation:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test ./cmd/ouro -run 'Test(RootCommandIncludesProductionCommands|CompletionAcceptCommand|CompletionStatusCommand)' -count=1 -v
```

passed.

Package validation:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test ./cmd/ouro -count=1
```

passed.
