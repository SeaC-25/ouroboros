# Milestone 75: Local Managed Binary Import

Date: 2026-06-05

## Summary

Added a workspace-local authoring bridge for operator-supplied managed binary payloads:

```sh
ouro toolchain managed-bundle-import <workspace> <name> <payload> --kind binary --version <version> --source <source> --import
```

The command copies a local regular file into:

```text
workspace/toolchain/managed-bundles/<name>/payload/<name>
```

It computes the SHA-256 digest with a streaming hash, writes the existing `bundle.yaml` contract, and re-reads the imported bundle through the normal managed bundle manifest and payload validator.

## Implemented

- Added `ImportManagedToolBundle` and `ManagedToolBundleImportOptions` / `ManagedToolBundleImportResult`.
- Added `toolchain managed-bundle-import` with an explicit `--import` write gate.
- Preserved the existing separate install boundary: the import does not create `workspace/toolchain/bin/<name>` or `workspace/toolchain/rules/<name>` links.
- Added app and CLI tests for successful import, missing `--import`, symlink payload rejection, directory payload rejection, digest/manifest output, and no payload execution.
- Updated README, development docs, findings, task plan, progress, and release archive README examples.

## Safety Boundary

The import path:

- accepts only a local filesystem payload path;
- accepts only `--kind binary` in this milestone;
- rejects symlink and directory payloads;
- rejects missing version/source metadata;
- refuses to overwrite an existing workspace bundle directory;
- cleans up a partially written bundle if manifest write or readback validation fails;
- writes only under `workspace/toolchain/managed-bundles/<name>/`.

It does not execute the payload, fetch URLs, download content, call package managers, run health probes, create final install links, modify pack manifests, write release-managed status-source markers, mutate replay/report state, contact providers/workers/endpoints/targets, or dispatch workers.

## Validation

Passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'ImportManagedToolBundle|ManagedBundleImport' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'ManagedToolBundle|ManagedBundle|ReleaseManaged|RuntimeTabs|NewTUIModelForWorkspaceShowsManagedBundleRowsReadOnly|RootCommandIncludesProductionCommands' -count=1 -v
```

Full repository validation is recorded in `progress.md` after final synchronization.

## Remaining Gaps

- Production third-party binary distribution and update provenance are not solved by this milestone.
- Expected-digest pinning for operator imports is implemented in milestone 76.
- Local rule-bundle authoring from operator-supplied directories is implemented in milestone 77.
- The IDE is still not a complete autonomous pentest environment; remaining work includes broader payload packaging, stronger model/runtime policy, and end-to-end operator workflows across planning, execution, replay, and reporting.
