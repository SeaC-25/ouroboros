# Milestone 128 - Managed Update Rule-Bundle Apply

Date: 2026-06-08

## Summary

Milestone 128 extends the managed update apply gate so it can apply already-staged `rule_bundle` directory payloads, in addition to the binary payload support from milestone 127:

```text
ouro toolchain apply-managed-update-download-execution <workspace> <download-execution-artifact> --apply
ouro toolchain managed-update-apply-executions <workspace> [artifact]
```

The gate still consumes a completed managed update download execution audit and still revalidates the linked request, plan, source URL, expected digest, actual digest, and staged payload before any workspace managed-bundle write. For `kind=rule_bundle`, the staged payload must already be a workspace-confined directory under the managed update download staging area.

## Safety Boundary

- Missing `--apply` fails before workspace bundle writes.
- Workspace readiness uses strict safety authorization.
- The source audit must be `status=completed`, `digest_status=matched`, `mode=execute`, and `action=download`.
- Supported apply kinds are `binary` and `rule_bundle`; unsupported kinds fail before writes.
- The linked pending update request and pending download plan are reread and must still match the download execution metadata.
- HTTPS source URL policy and the expected SHA-256 are revalidated.
- The staged payload path must resolve under `workspace/toolchain/managed-update-downloads/`.
- Binary staged payloads are regular files and are copied under `payload/<name>` with `0644` file mode.
- Rule-bundle staged payloads are directories, are inspected with the same managed import boundary, reject symlinks and non-regular entries, use canonical record v1 directory SHA-256, copy regular files under `payload/` with `0644` file mode, and write `bundle.yaml` `payload: payload`.
- Apply audits record `digest_algorithm=sha256` and `digest_scope=file|directory_canonical_records`.
- Apply audits live under `workspace/toolchain/managed-update-apply-executions/`.

This milestone does not create `toolchain/bin` or `toolchain/rules` links, install, extract archives, chmod executable, execute payloads, call package managers, probe health, contact providers/workers/endpoints/targets, mutate the original request/plan/download audit, or mutate replay/report state. Remote archive download, decompression, archive traversal checks, and archive provenance remain separate follow-up work.

## TUI

Workspace-bound Toolchain Health continues to render:

- `managed update apply executions: total=... completed=... failed=...`
- `managed update apply execution artifact: ...`
- `managed update apply execution digest: ...`

The artifact and digest rows now carry `digest_algorithm=sha256` plus `digest_scope=file|directory_canonical_records`. Runtime Tabs parses apply rows as `managed-update-apply-execution`, decodes apply audit, download execution, plan, request, source URL, release references, staged payload, workspace bundle, workspace manifest, workspace payload, install path, actor, reason, and error sidecars, and treats the action as read-only. Pressing `d` opens Inspector details; `r` and `x` remain unsupported.

## Validation

Focused validation passed before documentation synchronization:

```text
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'ApplyManagedToolBundleUpdateDownloadExecution|ManagedToolBundleUpdateApply' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'ApplyManagedToolBundleUpdateDownloadExecution|ManagedToolBundleUpdateApply|ApplyManagedUpdateDownloadExecution|ManagedUpdateApplyExecution|managed-update-apply-execution|DoesNotRequestReadOnlyRuntimeActions|NewTUIModelForWorkspaceShowsManagedUpdateApply' -count=1 -v
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
