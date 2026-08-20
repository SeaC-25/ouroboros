# Milestone 78: Local Managed Import Source Open Hardening

Date: 2026-06-05

## Summary

Hardened local managed bundle imports against source paths being replaced after `Lstat` inspection but before file reads or copies.

The import path now opens each source file, immediately calls `file.Stat()`, and verifies the opened file still matches the earlier `Lstat` result with `os.SameFile`. If the source path was replaced with a symlink or a different inode after inspection, the operation fails before a successful digest/copy and the workspace bundle directory is not retained.

## Implemented

- Added a stable source-open helper used by single-file binary digest, directory digest, binary copy, and rule-bundle directory copy paths.
- Added app regression tests that deterministically replace a binary source payload and a nested rule-bundle source file after inspection, proving both are rejected and no workspace bundle directory remains.
- Kept existing rule-bundle empty-directory rejection, symlink rejection, deterministic directory digest, and `0644` rule-file mode normalization.

## Safety Boundary

This hardening narrows the local source-tree race window without broadening import behavior. It does not execute payloads, fetch URLs, unpack archives, call package managers, run health probes, create final install links, edit pack manifests, write release-managed source markers, mutate replay/report state, contact providers/workers/endpoints/targets, or dispatch workers.

## Validation

Passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestImportManagedToolBundleRejects(BinaryPayload|RuleBundleFile)ReplacedAfterInspection' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'ImportManagedToolBundle|ManagedBundleImport' -count=1 -v
```

Broader validation is recorded in `progress.md` after final synchronization.

## Remaining Gaps

- Production third-party binary distribution and update provenance remain open.
- Release-managed expected-digest policy beyond manifest SHA-256 validation remains a future supply-chain policy layer.
- Fixed temporary manifest file hardening is implemented in milestone 79; canonical directory digest records are implemented in milestone 80.
- The IDE still needs broader end-to-end operator workflow closure before it is a complete autonomous pentest environment.
