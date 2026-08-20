# Milestone 84: Managed Bundle Update Metadata

Date: 2026-06-05

## Summary

Managed bundle manifests now support an optional offline `update:` block:

```yaml
update:
  latest_version: "1.3.0"
  latest_sha256: "<hex sha256 of payload>"
  checked_at: "2026-06-05T00:00:00Z"
  message: "current"
```

The app, CLI, and TUI expose `update_status` plus the recorded latest version, latest digest, checked time, and message for workspace-local bundles, release-managed bundles, release-managed status rows, and managed install plan/result output.

Status values are:

- `unknown`: no update metadata, empty update metadata, or no comparable latest version/digest.
- `current`: a valid latest version or latest digest matches the current manifest version/digest and no comparable field differs.
- `update_available`: a valid latest version or latest digest differs from the current manifest version/digest.
- `blocked`: invalid update metadata in release inventory/status rows.

The bundled `nuclei-templates` seed manifest now records update metadata while keeping the payload digest unchanged.

## Safety Boundary

Update metadata remains offline review metadata only. It is not a downloader, remote sync source, trust root, signature, expected digest pin, status-source policy, or workspace conflict criterion.

This milestone does not:

- Fetch latest versions from a network or local metadata source.
- Download, refresh, or synchronize managed bundles.
- Execute payloads or health probes.
- Copy bundles or create final links during list/status/TUI projection.
- Persist expected digest or update policy in the release-managed status-source marker.
- Treat update metadata differences as workspace managed-bundle conflicts.

## Validation

RED tests first showed missing update fields on app structs, CLI output, and TUI rows. A spec-review RED then showed workspace-local invalid update metadata needed a blocked list/TUI row rather than a generic list failure. After implementation, focused validation passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'UpdateMetadata|ManagedToolBundleManifestReportsOptionalUpdateMetadata|ReleaseManagedToolBundleBlocksInvalidUpdateMetadataWithoutWorkspaceWrites|ToolchainManagedBundleCommandsReportUpdateMetadata|NewTUIModelForWorkspaceShowsManagedBundleRowsReadOnly|NewTUIModelForWorkspaceShowsReleaseManagedStatusRowsFromSourceReadOnly' -count=1 -v
```

The blocked workspace-local row regression also passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'ListManagedToolBundlesReportsBlockedInvalidUpdateMetadata|ToolchainManagedBundleListReportsBlockedInvalidUpdateMetadata|NewTUIModelForWorkspaceShowsBlockedManagedBundleUpdateRowsReadOnly' -count=1 -v
```

Broader and full validation passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'ManagedToolBundle|ManagedBundle|ReleaseManaged|RuntimeTabs|NewTUIModelForWorkspaceShowsManagedBundleRowsReadOnly|NewTUIModelForWorkspaceShowsBlockedManagedBundleUpdateRowsReadOnly|NewTUIModelForWorkspaceShowsReleaseManagedStatusRowsFromSourceReadOnly|ModelToolPlanPreview|CompilePreviewReviewArtifact|RootCommandIncludesProductionCommands' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build
sh -n scripts/release.sh
git diff --check
gofmt -l internal/app/toolchain.go internal/app/toolchain_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go internal/tui/inspector.go cmd/ouro/main.go cmd/ouro/main_test.go release_scripts_test.go
```

Release smoke with a temporary `DIST_DIR` confirmed the packaged `nuclei-templates` release-managed status reports `provenance_status=recorded`, `update_status=current`, `update_latest_version=2026.06.04-ouro-seed`, `update_latest_sha256=01e5f879e780939e0d1a6bb0204045807a0585e8a5bb608032ae0e8b12c24b57`, `digest_status=verified`, and `digest_scope=directory_canonical_records`.

## Remaining Work

- Signed provenance and signature verification.
- Remote update/sync/download gates with explicit operator policy.
- Full upstream Nuclei template corpus packaging.
- Production third-party binary distribution.
- Broader row-value encoding for future whitespace-bearing display values.
