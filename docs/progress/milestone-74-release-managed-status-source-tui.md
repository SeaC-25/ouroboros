# Milestone 74: Release Managed Status Source TUI

Date: 2026-06-05

## Summary

Added a workspace-local release managed status source marker so the TUI can show the release/workspace import matrix without asking for a release root interactively:

- `SetReleaseManagedToolBundleStatusSource`, `ReadReleaseManagedToolBundleStatusSource`, and `ClearReleaseManagedToolBundleStatusSource` manage `workspace/toolchain/release-managed-status-source.json`.
- `ouro toolchain release-managed-status-source <workspace>` reads the marker.
- `ouro toolchain release-managed-status-source-set <workspace> <release-root> --set [--set-by] [--reason]` records the current release root and status counts.
- `ouro toolchain release-managed-status-source-clear <workspace> --clear [--cleared-by] [--reason]` rewrites the same marker to `status=cleared`.
- Workspace-bound Toolchain Health reads a current marker and renders `release managed status source:`, aggregate `release managed statuses:`, and per-bundle `release managed status:` rows.
- Runtime Tabs parse those rows as read-only actions, and `d` opens Inspector details for the marker, release root, release bundle manifest/payload paths, workspace bundle paths, final install path, and digest/import/link status.

This supersedes the Milestone 73 TUI gap only through a metadata marker. The TUI still does not provide an arbitrary release-root input form; operators choose the release root explicitly through the CLI marker gate.

Milestone 81 adds a separate CLI/app expected SHA-256 pin for named release-managed status and install commands. This milestone's status-source marker remains intentionally narrower: it stores the release root and matrix counts only, not an expected-pin policy, so TUI rows derived from the marker are unpinned release-root projections.

## Boundary

The set command reads the release/workspace status matrix to compute counts, then writes only the workspace-confined JSON marker. Clear rewrites only that marker.

The source read, status matrix read, TUI projection, and Runtime Tabs Inspector paths do not copy bundles, create symlinks, run payloads, execute health probes, call package managers, download content, contact providers, dispatch workers, contact endpoints/targets, or mutate replay/events/reports.

## Validation

Focused RED/GREEN validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'ReleaseManagedToolBundleStatusSource|ReleaseManagedStatusSource|ReleaseManagedStatusRows|DoesNotRequestReadOnlyRuntimeActions' -count=1 -v
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
gofmt -l internal/app/toolchain.go internal/app/toolchain_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go cmd/ouro/main.go cmd/ouro/main_test.go
sh -n scripts/release.sh
```

Release smoke confirmed the generated archive README includes `release-managed-status-source-set`, `release-managed-status-source`, and `nuclei-templates`.
