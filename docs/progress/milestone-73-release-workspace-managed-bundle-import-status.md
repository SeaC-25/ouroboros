# Milestone 73: Release Workspace Managed Bundle Import Status

Date: 2026-06-05

## Summary

Added a read-only release/workspace managed bundle import-status matrix:

- `ListReleaseManagedToolBundleStatuses(root, releaseRoot)` and `ReadReleaseManagedToolBundleStatus(root, releaseRoot, name)` join release-side bundle metadata with current workspace copy/link state.
- `ouro toolchain release-managed-status <workspace> <release-root> [name]` lists or inspects the same status matrix without changing the existing release-only `release-managed-bundles <release-root> [name]` command.
- Status rows expose `digest_status=verified|blocked`, `digest_algorithm=sha256`, `digest_scope=file|directory_canonical_records`, `import_status=not_imported|imported|linked|conflict|blocked`, and `link_status=missing|linked|conflict|blocked`.
- `not_imported` means the release bundle digest is verified but no workspace copy/link exists; `imported` means the workspace copy verifies but the final link is missing; `linked` means the final workspace link targets the expected workspace-managed payload; `conflict` means an existing workspace bundle or final install path disagrees with the release bundle; `blocked` means release/workspace manifest, payload, digest, or path validation failed.

## Boundary

The status matrix is an operator visibility layer only. It does not copy bundles, create symlinks, run payloads, execute health probes, call package managers, download content, contact providers, dispatch workers, contact endpoints/targets, or mutate replay/events/reports.

Release inventory remains release-root only. Workspace-bound Toolchain Health continues to show imported workspace-local `managed bundle:` rows; release-root-aware TUI discovery remains a later UI input-design slice.

## Validation

Focused RED/GREEN validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'ReleaseManagedToolBundleStatus|ReleaseManagedStatusCommand' -count=1 -v
```

Final validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct VERSION=0.1.0-dev DIST_DIR=<tmp> ./scripts/release.sh
tar -xOf <tmp>/ouro_0.1.0-dev_linux_amd64.tar.gz ouro_0.1.0-dev_linux_amd64/README.md
git diff --check
gofmt -l internal/app/toolchain.go internal/app/toolchain_test.go cmd/ouro/main.go cmd/ouro/main_test.go
```

Release smoke confirmed the generated archive README includes both `release-managed-status` and `nuclei-templates`.
