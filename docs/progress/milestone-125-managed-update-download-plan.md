# Milestone 125: Managed Update Download Plan Artifacts

Date: 2026-06-07

## Summary

Milestone 125 adds a metadata-only download-source review artifact for managed bundle updates:

```sh
ouro toolchain managed-update-download-plan <workspace> <request-artifact> --plan --source-url <https-url> --expected-sha256 <sha256>
ouro toolchain managed-update-download-plans <workspace> [artifact]
```

The artifact is derived from an existing pending managed update request and records the reviewed remote source URL, expected SHA-256, release/request references, actor, timestamp, update message, and reason.

## Scope

- Create `workspace/toolchain/managed-update-download-plans/*.json` as independent pending metadata-only review artifacts.
- Require the explicit `--plan` gate before writing.
- Accept only workspace-confined `.json` update request artifacts.
- Require request `status=pending`, `mode=metadata-only`, and `update_status=update_available`.
- Require an absolute HTTPS `--source-url` with a host.
- Reject source URLs with userinfo, query strings, fragments, or whitespace.
- Require a 64-character hex `--expected-sha256` and normalize it to lowercase.
- When the request records `latest_sha256`, require the expected digest to match it.
- Keep the original request artifact unchanged.
- List/read plan artifacts with `managed-update-download-plans`.
- Render workspace-bound Toolchain Health read-only summary and `managed update download plan artifact:` rows.
- Decode exact request/source/release/text sidecars in Runtime Tabs for read-only Inspector details.

## Boundaries

This milestone is not a downloader, updater, sync worker, installer, or package-manager path.

It does not download payloads, fetch remote metadata, copy release bundles, create final links, install tools, execute payloads, run health probes, call package managers, contact providers, dispatch workers, contact endpoints or targets, mutate the original update request, or mutate replay/report state.

The TUI exposes only read-only details for these rows. `r` and `x` remain unsupported.

## Validation

RED validation first failed on missing app APIs, CLI commands, and RuntimeAction parsing:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'ManagedToolBundleUpdateDownloadPlan|ManagedUpdateDownloadPlan|managed-update-download-plan|RuntimeManagedUpdateDownloadPlan|NewTUIModelForWorkspaceShowsManagedUpdateDownloadPlan|RootCommandIncludesProductionCommands' -count=1 -v
```

Focused GREEN validation passed after implementation and the width-stable TUI assertion fix:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'ManagedToolBundleUpdateDownloadPlan|ManagedUpdateDownloadPlan|managed-update-download-plan|RuntimeManagedUpdateDownloadPlan|NewTUIModelForWorkspaceShowsManagedUpdateDownloadPlan|RootCommandIncludesProductionCommands' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -count=1
```

Final validation passed after documentation synchronization:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-release-managed
sh -n scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_release_managed.sh
git diff --check
gofmt -l internal/app/toolchain.go internal/app/toolchain_test.go cmd/ouro/main.go cmd/ouro/main_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go
```
