# Milestone 112 - Managed Release Path Reference Row-Value Encoding

## Summary

Milestone 112 hardens parser-consumed managed-bundle and release-managed path reference rows:

- `managed bundle:`
- `release managed status source:`
- `release managed status:`

The rows now keep compact readable companions and add RawURL base64 sidecars for managed payload, install, and manifest paths; release status-source root and marker paths; and release/workspace bundle, payload, manifest, and install references consumed by RuntimeAction and Inspector metadata.

## Why

These Toolchain Health rows feed read-only IDE details for managed payload and release-managed bundle state. Before this milestone, path references were still parsed from whitespace-delimited row values. Paths containing spaces, tabs, repeated whitespace, or embedded tokens such as `status=`, `mode=`, `digest_status=`, `import_status=`, `link_status=`, `release_manifest=`, `workspace_manifest=`, or `install=` could truncate values or retarget parsed Toolchain Health metadata.

## Changes

- Added `payload_b64=`, `install_b64=`, and `manifest_b64=` to workspace-local `managed bundle:` rows.
- Added `release_root_b64=` and `marker_b64=` to `release managed status source:` rows while preserving `reason_b64=`.
- Added `release_bundle_b64=`, `release_payload_b64=`, `release_manifest_b64=`, `workspace_bundle_b64=`, `workspace_payload_b64=`, `workspace_manifest_b64=`, and `install_b64=` to `release managed status:` rows while preserving `source_b64=` and `error_b64=`.
- Updated Runtime Tabs managed/release parsers to strip legacy readable path fields before machine-field parsing and decode exact path references from sidecars.
- Added release bundle path exposure to `RuntimeAction` and Node Inspector details so `release_bundle_b64=` is not only parsed defensively but also visible to operators.
- Preserved simple no-sidecar legacy fallback behavior for older rows.

## Validation

Passed before documentation synchronization:

- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Test(ManagedBundleProjectionIncludesEncodedReferenceFields|ReleaseManagedStatusProjectionIncludesEncodedReferenceFields|ReleaseManagedStatusSourceProjectionIncludesEncodedReferenceFields)' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntime(ManagedBundle|ReleaseManagedStatus).*EncodedReferenceFields' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'ManagedBundle|ReleaseManaged|Toolchain|RuntimeTabs|RuntimeAction|RootCommandIncludesProductionCommands' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh scripts/build.sh scripts/install.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go internal/tui/inspector.go`

## Review

A read-only subagent audit found that `release_bundle_b64=` was emitted and stripped but not surfaced as a RuntimeAction detail. The implementation now maps that sidecar to `RuntimeAction.ReleaseBundlePath`, renders `release bundle path:` in the Inspector, and extends release-managed parser coverage for that field.

## Follow-Ups

Remaining row-value work is broader display/message rows only if a RuntimeAction parser begins consuming them, plus any future parser-consumed rows found by audit.

No release smoke was rerun before this documentation update because release packaging and bundled manifests were not touched.
