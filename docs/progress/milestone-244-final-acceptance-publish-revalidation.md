# Milestone 244: Final Acceptance Publish Revalidation

## Summary

Hardened `ouro completion-accept` so the final local acceptance artifact is not published after the reviewed log directory or final output path has been replaced.

After writing and closing the same-directory temporary `completion-final-acceptance.txt` file, the command now revalidates:

- the completion audit log directory path
- the final acceptance output path

Only then does it rename the temporary file into place. The same canonical absolute path and no-symlink-component rules from milestone 243 apply at this final publication boundary.

## Boundary

This is local filesystem hardening for the mutable final acceptance file. It is not a signature, append-only store, directory-fd based no-follow publication primitive, remote attestation, or replacement for real operator evidence review.

## Validation

RED:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test ./cmd/ouro -run TestCompletionAcceptCommandRevalidatesFinalAcceptancePathBeforePublish -count=1 -v
```

failed before implementation because the publish-time hook did not exist, proving there was no deterministic publication-boundary revalidation point.

GREEN:

```bash
GOPROXY=https://goproxy.cn,direct TMPDIR=/home/zss/t GOTMPDIR=/home/zss/t GOCACHE=$PWD/.cache/go-build go test ./cmd/ouro -run TestCompletionAcceptCommandRevalidatesFinalAcceptancePathBeforePublish -count=1 -v
```

passed after adding publication-boundary revalidation. The test moves the reviewed log directory aside, replaces it with a symlink to an outside directory just before publish, and verifies `completion-accept` fails before `rename` without writing `completion-final-acceptance.txt` to the replacement target.

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
