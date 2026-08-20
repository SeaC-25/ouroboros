# Milestone 126 - Managed Update Download Execution Audit

Date: 2026-06-07

## Summary

Milestone 126 adds the explicit managed update download execution gate:

```text
ouro toolchain execute-managed-update-download-plan <workspace> <plan-artifact> --execute
ouro toolchain managed-update-download-executions <workspace> [artifact]
```

The gate consumes an existing pending metadata-only managed update download plan, revalidates the linked pending update request, downloads the reviewed HTTPS payload into workspace-local staging, verifies SHA-256, and writes a completed or failed audit artifact.

## Safety Boundary

- Missing `--execute` fails before network, staging, or audit writes.
- Workspace readiness uses strict safety authorization.
- The plan and linked request must remain pending, metadata-only, and still match all request-derived metadata.
- HTTPS source URL and expected SHA-256 are revalidated at execution time.
- Redirects are rejected, HTTP status must be `200`, and payload size is capped by both `Content-Length` and streaming limit.
- Bytes stream to a temp file under `workspace/toolchain/managed-update-downloads/`.
- The final `payload` file is published only after the computed SHA-256 matches the expected digest.
- Digest mismatch removes temp/payload bytes and writes a failed audit with the actual SHA-256.
- Execution audits live under `workspace/toolchain/managed-update-download-executions/`.

This milestone does not copy into `managed-bundles`, create `toolchain/bin` or `toolchain/rules` links, install, extract, chmod executable, execute payloads, call package managers, probe health, contact providers/workers/endpoints/targets, mutate the original request or plan, or mutate replay/report state.

## TUI

Workspace-bound Toolchain Health now renders:

- `managed update download executions: total=... completed=... failed=...`
- `managed update download execution artifact: ...`
- `managed update download execution digest: ...`

Runtime Tabs parses execution rows as `managed-update-download-execution`, decodes audit, plan, request, source URL, release references, staged payload, actor, reason, and error sidecars, and treats the action as read-only. Pressing `d` opens Inspector details; `r` and `x` remain unsupported.

## Validation

Focused validation passed:

```text
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'ExecuteManagedToolBundleUpdateDownloadPlan|ManagedToolBundleUpdateDownloadExecution|ExecuteManagedUpdateDownloadPlan|managed-update-download-execution|execute-managed-update-download-plan|RootCommandIncludesProductionCommands' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -run 'ManagedUpdateDownloadExecution|RuntimeManagedUpdateDownloadExecution|NewTUIModelForWorkspaceShowsManagedUpdateDownloadExecution' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'ManagedToolBundleUpdateDownloadExecution|ExecuteManagedToolBundleUpdateDownloadPlan|ManagedUpdateDownloadExecution|managed-update-download-execution|execute-managed-update-download-plan|RootCommandIncludesProductionCommands|NewTUIModelForWorkspaceShowsManagedUpdateDownloadExecution|RuntimeManagedUpdateDownloadExecution' -count=1 -v
```

Final validation passed:

```text
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-release-managed
sh -n scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_release_managed.sh
git diff --check
gofmt -l internal/app/toolchain.go internal/app/toolchain_test.go cmd/ouro/main.go cmd/ouro/main_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go
```
