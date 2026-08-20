# Milestone 77: Local Managed Rule Bundle Import

Date: 2026-06-05

## Summary

Extended the workspace-local managed bundle import bridge to operator-supplied rule-bundle directories:

```sh
ouro toolchain managed-bundle-import <workspace> nuclei-templates ./nuclei-templates --kind rule_bundle --version 2026.06.05 --source operator-local/nuclei-templates --expected-sha256 <sha256> --import
```

`--kind binary` keeps its existing regular-file behavior. `--kind rule_bundle` accepts a local directory, computes the same deterministic directory SHA-256 used by managed bundle readback, copies the directory into `workspace/toolchain/managed-bundles/<name>/payload/`, writes `payload: payload` in `bundle.yaml`, and re-reads the bundle through the normal manifest/digest validator.

## Implemented

- Allowed `managed-bundle-import` to accept `--kind rule_bundle`.
- Reused the managed directory digest contract with empty directories rejected. Milestone 80 supersedes the original `relative-path + NUL + file-bytes + NUL` encoding with canonical record v1.
- Reused `--expected-sha256` for rule-bundle directories, including uppercase normalization and mismatch rejection before workspace bundle writes.
- Copied rule-bundle directories into the existing managed bundle shape with copied file permissions normalized to `0644` and no final `toolchain/rules/<name>` link.
- Updated CLI help, README, development docs, findings, release README generation, task plan, and prior milestone follow-up wording.

## Safety Boundary

The import path rejects top-level symlink payloads, `binary` directory payloads, `rule_bundle` file payloads, empty rule-bundle directories, directory-internal symlinks, non-regular files, invalid or mismatched expected digests, missing version/source metadata, unsupported kinds, missing `--import`, and existing workspace bundle directories.

The rule-bundle import path still does not execute payloads, fetch URLs, download content, unpack archives, call package managers, run health probes, create final install links, edit pack manifests, write release-managed source markers, mutate replay/report state, contact providers/workers/endpoints/targets, or dispatch workers.

## Validation

Passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'ImportManagedToolBundle|ManagedBundleImport' -count=1 -v
```

Broader validation is recorded in `progress.md` after final synchronization.

## Remaining Gaps

- Production third-party binary distribution and update provenance remain open.
- Release-managed expected-digest policy beyond manifest SHA-256 validation remains a future supply-chain policy layer.
- Richer rule-corpus provenance, signatures, and upstream synchronization remain separate from this local directory authoring bridge.
- Source path replacement after inspection is hardened in milestone 78, fixed-temp-file hardening is implemented in milestone 79, and canonical directory digest records are implemented in milestone 80.
- The IDE still needs broader end-to-end operator workflow closure before it is a complete autonomous pentest environment.
