# Milestone 123: Managed Update Request Artifacts

Date: 2026-06-07

## Summary

Milestone 123 turns release-managed offline `update_status=update_available` metadata into an auditable pending operator request artifact.

Operators can run:

```sh
ouro toolchain managed-update-request <workspace> <release-root> <name> --request --requested-by operator --reason "review update"
ouro toolchain managed-update-requests <workspace>
ouro toolchain managed-update-requests <workspace> <artifact>
```

The artifact lives under `workspace/toolchain/managed-update-requests/` and records current/latest version and SHA-256 metadata, release root, release bundle/manifest/payload paths, request actor/time, update message, and reason.

## Scope

- Creation requires the explicit `--request` flag.
- Creation accepts only a valid release-managed bundle with `update_status=update_available`.
- Missing release bundles, invalid release bundle metadata, and `unknown`, `current`, or `blocked` update states fail before artifact writes.
- List/read commands expose request artifacts without mutating workspace state.
- Workspace-bound Toolchain Health renders a request summary, `managed update request artifact:` rows, and compact digest rows.
- RuntimeAction treats request rows as `kind=managed-update-request` and read-only.
- Path and text values are carried with encoded sidecars for artifact, release root, release bundle, release manifest, release payload, requested-by, update message, and reason fields.

## Boundaries

This milestone is metadata-only. It does not download, sync, copy, link, install, execute payloads, call package managers, probe health, contact providers, dispatch workers, contact endpoints or targets, or mutate replay/report state.

The request artifact is not a trust root, remote update policy, signature policy, expected-digest policy, or automatic updater. It records operator intent for later review work.

## Validation

Focused RED/GREEN validation passed before documentation synchronization:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'ManagedToolBundleUpdateRequest|ManagedUpdateRequest|managed-update-request|RuntimeManagedUpdateRequest|NewTUIModelForWorkspaceShowsManagedUpdateRequest|DoesNotRequestReadOnlyRuntimeActions' -count=1 -v
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
