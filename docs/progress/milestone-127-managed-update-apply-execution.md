# Milestone 127 - Managed Update Apply Execution

Date: 2026-06-08

## Summary

Milestone 127 adds the explicit managed update apply gate:

```text
ouro toolchain apply-managed-update-download-execution <workspace> <download-execution-artifact> --apply
ouro toolchain managed-update-apply-executions <workspace> [artifact]
```

The gate consumes a completed managed update download execution audit, revalidates the linked request, plan, source URL, expected digest, actual digest, and staged payload, then copies the verified binary payload into the workspace managed-bundle directory and writes an apply audit.

## Safety Boundary

- Missing `--apply` fails before workspace bundle writes.
- Workspace readiness uses strict safety authorization.
- The source audit must be `status=completed`, `digest_status=matched`, `mode=execute`, `action=download`, and `kind=binary`.
- The linked pending update request and pending download plan are reread and must still match the download execution metadata.
- HTTPS source URL policy and the expected SHA-256 are revalidated.
- The staged payload path must resolve under `workspace/toolchain/managed-update-downloads/`.
- The staged payload is rehashed before workspace copy, and the digest must match the expected digest.
- The workspace bundle is replaced through a temp bundle directory and final bundle readback.
- The copied binary payload is written under `workspace/toolchain/managed-bundles/<name>/payload/<name>` with `0644` file mode.
- The refreshed `bundle.yaml` records the latest version, source, and SHA-256.
- Apply audits live under `workspace/toolchain/managed-update-apply-executions/`.

This milestone does not create `toolchain/bin` or `toolchain/rules` links, install, extract, chmod executable, execute payloads, call package managers, probe health, contact providers/workers/endpoints/targets, mutate the original request/plan/download audit, or mutate replay/report state. Rule-bundle update application remains a separate packaged/extract/provenance follow-up.

## TUI

Workspace-bound Toolchain Health now renders:

- `managed update apply executions: total=... completed=... failed=...`
- `managed update apply execution artifact: ...`
- `managed update apply execution digest: ...`

Runtime Tabs parses apply rows as `managed-update-apply-execution`, decodes apply audit, download execution, plan, request, source URL, release references, staged payload, workspace bundle, workspace manifest, workspace payload, install path, actor, reason, and error sidecars, and treats the action as read-only. Pressing `d` opens Inspector details; `r` and `x` remain unsupported.

## Validation

Focused validation passed before documentation synchronization:

```text
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'ApplyManagedToolBundleUpdateDownloadExecution|ManagedToolBundleUpdateApply' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'RootCommandIncludesProductionCommands|ApplyManagedUpdateDownloadExecution|ManagedUpdateApply' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -run 'ManagedUpdateApplyExecution|managed-update-apply-execution|NewTUIModelForWorkspaceShowsManagedUpdateApply|DoesNotRequestReadOnlyRuntimeActions' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'ApplyManagedToolBundleUpdateDownloadExecution|ManagedToolBundleUpdateApply|ApplyManagedUpdateDownloadExecution|ManagedUpdateApplyExecution|managed-update-apply-execution|RootCommandIncludesProductionCommands|NewTUIModelForWorkspaceShowsManagedUpdateApply|DoesNotRequestReadOnlyRuntimeActions' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -count=1
```

Final validation passed:

```text
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-release-managed
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct sh -n scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_release_managed.sh
git diff --check
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct gofmt -l internal/app/toolchain.go internal/app/toolchain_test.go cmd/ouro/main.go cmd/ouro/main_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go
```
