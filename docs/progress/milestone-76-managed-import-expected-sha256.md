# Milestone 76: Managed Import Expected SHA-256

Date: 2026-06-05

## Summary

Added optional expected digest pinning to local managed binary imports:

```sh
ouro toolchain managed-bundle-import <workspace> <name> <payload> --kind binary --version <version> --source <source> --expected-sha256 <sha256> --import
```

When `--expected-sha256` is supplied, Ouroboros validates the value as a 64-character SHA-256 hex digest, normalizes it to lowercase, computes the local payload digest with a streaming hash, and compares both values before creating the workspace managed bundle directory.

## Implemented

- Added `ExpectedSHA256` / `ExpectedSHA256Provided` to `ManagedToolBundleImportOptions`.
- Added `ExpectedSHA256` to `ManagedToolBundleImportResult`.
- Added `--expected-sha256` to `toolchain managed-bundle-import`.
- Added CLI output field `expected_sha256=<digest>` after the computed `sha256=<digest>`.
- Added app and CLI tests for matching uppercase expected digests, invalid expected digests, explicit empty CLI values, mismatch failures, and no workspace bundle writes on failure.

## Safety Boundary

Invalid or mismatched expected digests fail before:

- creating `workspace/toolchain/managed-bundles/<name>/`;
- copying payload bytes;
- writing `bundle.yaml` or temp manifest files;
- creating final install links.

The path still does not execute payloads, fetch URLs, download content, call package managers, run health probes, edit pack manifests, write release-managed source markers, mutate replay/report state, contact providers/workers/endpoints/targets, or dispatch workers.

## Validation

Passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'ImportManagedToolBundle|ManagedBundleImport' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'ManagedToolBundle|ManagedBundle|ReleaseManaged|RuntimeTabs|NewTUIModelForWorkspaceShowsManagedBundleRowsReadOnly|RootCommandIncludesProductionCommands' -count=1 -v
```

Full repository validation is recorded in `progress.md` after final synchronization.

## Remaining Gaps

- Production third-party binary distribution and update provenance remain open.
- Local rule-bundle authoring from operator-supplied directories is implemented in milestone 77.
- Release-managed expected-digest policy beyond manifest SHA-256 validation remains a future supply-chain policy layer.
