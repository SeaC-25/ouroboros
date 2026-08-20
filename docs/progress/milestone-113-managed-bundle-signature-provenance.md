# Milestone 113 - Managed Bundle Signature Provenance

## Summary

Milestone 113 adds optional offline signature metadata for managed tool bundles and release-managed bundles.

Bundle manifests may now include a `signature:` block. The app verifies an Ed25519 signature over a fixed bundle identity message containing:

- `name`
- `kind`
- `version`
- `payload_sha256`

Verified signatures surface as `signature_status=verified`; unsigned bundles surface as `signature_status=missing`; invalid signatures fail strict read/install paths and appear as `signature_status=blocked` in tolerant inventory/status projections.

## Why

Managed bundles were already digest-checked, expected-digest pinnable, provenance-visible, and update-visible, but operators still had no offline signature consistency field to review before importing or linking release-managed payloads. This milestone narrows that gap without adding a downloader, keyserver, package manager, remote trust policy, or automatic update path.

## Changes

- Added `signature:` manifest parsing and Ed25519 verification for workspace-local and release-side managed bundles.
- Added signature metadata fields to managed bundle, release-managed bundle/status, managed install-plan/result, and release-managed install-plan/result app structures.
- Surfaced `signature_status`, `signature_algorithm`, `signature_key_id`, `signature_signed_at`, and `signature_signed_by` in CLI inventory/status/install-plan/install output.
- Surfaced the same signature metadata in workspace-bound Toolchain Health rows, RuntimeAction details, and Node Inspector details.
- Kept unsigned bundles compatible by reporting `signature_status=missing`.
- Kept invalid signatures fail-closed for strict read/install APIs before workspace copy/link writes.
- Preserved tolerant release inventory/status list behavior by reporting blocked rows instead of aborting the entire list.

## Boundaries

This is an offline consistency check and review surface. It does not:

- establish a remote trust root;
- fetch keys or payloads;
- download or update managed bundles;
- replace expected SHA-256 pins;
- execute payloads or health probes;
- contact providers, workers, endpoints, or targets;
- mutate replay/report state.

The embedded public key, key id, timestamp, and signer fields are review metadata. `release-managed-status-source-set --set` remains an explicit local marker write for TUI release-root selection; blocked per-bundle signatures still keep import/link/install blocked.

## Validation

Passed before documentation synchronization:

- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run TestToolchainManagedBundleCommandsReportSignatureMetadata -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro ./internal/app ./internal/tui -run 'TestToolchainManagedBundleCommandsReportSignatureMetadata|TestManagedBundleProjectionIncludesSignatureMetadata|TestRuntimeManagedBundleShowsSignatureMetadata|TestManagedToolBundleManifestVerifiesOptionalSignatureMetadata|TestReleaseManagedToolBundleBlocksInvalidSignatureMetadataWithoutWorkspaceWrites' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'Signature|ManagedBundle|ReleaseManaged' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh scripts/build.sh scripts/install.sh`
- `git diff --check`
- `gofmt -l internal/app/toolchain.go internal/app/toolchain_test.go cmd/ouro/main.go cmd/ouro/main_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Review

A read-only subagent audit found no blocking propagation or write-boundary issue. It recommended two extra coverage points, both added:

- invalid release-managed signatures reject `ExecuteReleaseManagedToolBundleInstall(... Execute:true)` before copying workspace bundles or creating install links;
- release-managed status RuntimeAction and Inspector details include the signature metadata fields.

## Follow-Ups

The full IDE target still needs production third-party payload distribution, remote trust/download/update policy design, broader packaged rule-corpus provenance, and end-to-end workflow closure. Those are separate milestones from this offline signature metadata slice.
