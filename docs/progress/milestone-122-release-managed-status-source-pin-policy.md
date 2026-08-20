# Milestone 122: Release Managed Status-Source Pin Policy

Date: 2026-06-07

## Summary

Milestone 122 lets a workspace release-managed status-source marker persist one local expected SHA-256 policy for a named release-managed bundle.

Operators can now run:

```sh
ouro toolchain release-managed-status-source-set <workspace> <release-root> --set --pin-bundle <name> --expected-sha256 <sha256>
```

The marker records `expected_sha256_bundle`, normalized `expected_sha256`, and `expected_sha256_status`. Workspace-bound Toolchain Health applies that marker policy when it renders release-managed source, summary, and per-bundle status rows.

## Scope

- `--pin-bundle` and `--expected-sha256` are required together.
- Expected SHA-256 values are validated as 64-character hex and normalized to lowercase.
- A missing pinned bundle fails the marker write instead of silently persisting an unmatched policy.
- Matching pins keep the normal read-only release/workspace status counts.
- Mismatched pins persist `expected_sha256_status=mismatch`, count only the pinned bundle as blocked, keep `digest_status=verified` when manifest/payload validation succeeds, and avoid workspace copy/link/install side effects.
- RuntimeAction and Inspector details expose the pin policy through source/status hints.

## Implementation

- `internal/app/toolchain.go`
  - Extends `ReleaseManagedToolBundleStatusSource` and options with expected pin fields.
  - Adds a pinned release-managed status matrix helper used by both status-source writes and TUI projections.
  - Persists source-level expected pin metadata in `workspace/toolchain/release-managed-status-source.json`.

- `cmd/ouro/main.go`
  - Adds `release-managed-status-source-set --pin-bundle` and `--expected-sha256`.
  - Prints expected pin fields from set/read output.

- `internal/app/tui_snapshot.go`
  - Applies the persisted marker pin when projecting release-managed status rows.
  - Keeps pin, digest, import, link, and update status fields early enough to stay visible in normal-width Toolchain Health rows.

- `internal/tui/runtime_tabs.go`
  - Adds expected pin hints for release-managed source and per-bundle status actions.

## Boundaries

This milestone is a local metadata policy only. It does not download, sync, install, copy, link, execute payloads, call package managers, probe health, contact providers, dispatch workers, contact endpoints or targets, or mutate replay/report state.

It does not make provenance/update metadata into trust roots or remote update policies. It also does not replace release manifest/payload self-validation; a valid payload with a mismatched operator pin still reports `digest_status=verified`.

## Validation

Focused RED/GREEN validation passed:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestReleaseManagedToolBundleStatusSourcePersistsExpectedPinPolicy' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'TestToolchainReleaseManagedStatusSourceSetPersistsExpectedPinPolicy' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAppliesReleaseManagedStatusSourcePinPolicy' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeReleaseManagedStatus(SourceShowsExpectedPinPolicy|ShowsExpectedPinPolicy)' -count=1 -v
```

Related package validation passed:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1
```

Final validation passed after documentation synchronization:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow
sh -n scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh
git diff --check
gofmt -l internal/app/toolchain.go internal/app/toolchain_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go cmd/ouro/main.go cmd/ouro/main_test.go
```
