# Milestone 129 - Managed Update Rule-Bundle Archive Download

Date: 2026-06-08

## Summary

Milestone 129 extends the managed update download gate so `rule_bundle` updates can be fetched as reviewed `tar.gz` archives while preserving separate trust scopes:

```text
ouro toolchain managed-update-download-plan <workspace> <request-artifact> --plan --source-url <https-url> --expected-sha256 <directory-sha256> --archive-format tar.gz --expected-archive-sha256 <archive-sha256>
ouro toolchain execute-managed-update-download-plan <workspace> <plan-artifact> --execute
ouro toolchain apply-managed-update-download-execution <workspace> <download-execution-artifact> --apply
```

For `binary`, `expected_sha256` remains the staged file digest. For `rule_bundle`, `expected_sha256` is the extracted canonical record v1 directory digest, while `archive_expected_sha256` is the downloaded archive byte digest.

## Safety Boundary

- Download plans remain metadata-only and require `--plan`.
- Rule-bundle archive plans currently support `archive_format=tar.gz`.
- Download execution requires `--execute`, rereads the pending plan and request, rejects redirects and non-200 responses, enforces bounded streaming, and writes only under `workspace/toolchain/managed-update-downloads/`.
- Archive bytes are written as `archive`, verified with SHA-256, and only then extracted.
- Extraction rejects path traversal, absolute paths, symlinks, hardlinks, devices, and non-regular entries.
- Extracted rule files are normalized to `0644` and published under staged `payload/` only after the extracted directory digest matches `expected_sha256`.
- Download audits record payload `digest_algorithm=sha256`, `digest_scope=directory_canonical_records`, plus archive `archive_digest_algorithm=sha256` and `archive_digest_scope=file`.
- Apply revalidates archive format, expected/actual archive SHA-256, `archive_digest_status=matched`, and archive digest scope when the plan declared archive metadata.

This milestone does not create final `toolchain/rules` links, install, chmod executable, run payloads, call package managers, probe health, contact providers/workers/endpoints/targets, mutate the original request/plan/download audit, or mutate replay/report state.

## TUI

Workspace-bound Toolchain Health now renders archive metadata on managed update download plan and execution rows:

- `managed update download plan artifact: ... archive_format=... archive_expected_sha256=... archive_expected_sha256_status=...`
- `managed update download execution artifact: ... archive=... archive_b64=... archive_digest_status=... archive_digest_scope=file ...`
- `managed update download execution digest: ... digest_scope=directory_canonical_records archive_digest_status=... archive_digest_scope=file ...`
- `managed update download execution transfer: ... bytes_written=... http_status=... archive_format=...`

Runtime Tabs decodes archive sidecars into read-only Inspector details. Pressing `d` opens details; `r` and `x` remain unsupported for request, plan, download execution, and apply execution rows.

## Validation

Focused validation passed before final repository validation:

```text
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'RuleBundleArchive|UnsafeRuleBundleArchive|TamperedArchiveMetadata|ManagedToolBundleUpdateDownloadPlan|ManagedToolBundleUpdateDownloadExecution|ExecuteManagedUpdateDownloadPlan|ManagedUpdateDownload|RuntimeManagedUpdateDownload|NewTUIModelForWorkspaceShowsManagedUpdateDownload|DoesNotRequestReadOnlyRuntimeActions|RootCommandIncludesProductionCommands' -count=1 -v
```

Final validation passed:

```text
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-release-managed
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct sh -n scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_release_managed.sh
git diff --check
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct gofmt -l internal/app/toolchain.go internal/app/toolchain_test.go cmd/ouro/main.go cmd/ouro/main_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go internal/tui/inspector.go
```
