# Milestone 72: Managed Bundle Discovery Status

Date: 2026-06-05

## Summary

Added a read-only managed bundle discovery/status slice:

- `ouro toolchain release-managed-bundles <release-root> [name]` lists or inspects release-side managed bundles before workspace import.
- Release inventory lists valid bundles as `status=available` and keeps invalid bundles visible as `status=blocked` with digest/path errors.
- Workspace-bound Toolchain Health now renders imported workspace-local bundles as read-only `managed bundle:` rows with Inspector details.
- Release-managed install now checks an existing final `workspace/toolchain/bin|rules/<name>` path before copying the release bundle into the workspace, and cleans up a copied bundle if the later link step fails.

## Boundary

This milestone does not download bundles, run payloads, execute health probes, call package managers, contact providers, dispatch workers, contact endpoints/targets, or automatically import release bundles. `release-managed-bundles` is release inventory only; the combined workspace+release import-status matrix with `not_imported`, `imported`, `linked`, and `conflict` states remains a follow-up.

## Validation

Focused validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'ReleaseManagedToolBundle|ListReleaseManagedToolBundles|NewTUIModelForWorkspaceShowsManagedBundleRowsReadOnly|ToolchainReleaseManagedBundlesCommand|ToolchainReleaseManagedInstallPlanAndInstallCommands|DoesNotRequestReadOnlyRuntimeActions' -count=1 -v
```

Final validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct DIST_DIR=<tmp> VERSION=0.1.0-dev ./scripts/release.sh
git diff --check
gofmt -l internal/app/toolchain.go internal/app/toolchain_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go cmd/ouro/main.go cmd/ouro/main_test.go
```
