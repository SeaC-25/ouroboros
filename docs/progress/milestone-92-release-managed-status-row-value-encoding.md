# Milestone 92: Release Managed Status Row-Value Encoding

Milestone 92 extends parser-safe row-value sidecars to workspace-bound Toolchain Health `release managed status:` rows.

## Scope

Release-managed status projections now retain the compact human-readable fields and add RawURL base64 sidecars for:

- `source_b64`
- `error_b64`

Runtime Tabs now strips legacy `source` and `error` text before parsing release-managed machine metadata, then decodes the sidecars for Inspector/action details. This prevents embedded `name=`, `status=`, `mode=`, `install=`, `release_manifest=`, `workspace_manifest=`, `digest_status=`, `import_status=`, `link_status=`, or fake sidecar markers in source/error text from retargeting the read-only Inspector action.

## Compatibility

The readable `source=` and `error=` fields remain in each row for existing human-readable output. Legacy rows without sidecars still fall back to the legacy row parser after source/error cleanup, so older snapshots remain inspectable.

This is only a row-format compatibility layer. It does not copy release-managed bundles, create links, run payloads, execute health probes, call package managers, download content, contact providers/workers/endpoints/targets, mutate replay/report state, add signed provenance, or persist a pinned release-managed status policy.

## Validation

RED parser tests first reproduced release-managed source/error truncation and metadata retargeting. RED projection tests also proved the rows lacked `source_b64` and `error_b64` sidecars. GREEN added sidecar emission, parser isolation through source/error-stripped machine rows, and sidecar decoding with legacy fallback.

A later quality review found old no-sidecar `release managed status:` rows could overread `source` into following release-managed fields because the generic runtime field boundary list did not include `sha256`, digest/import/link fields, manifest paths, or `install`. A RED legacy-row test reproduced the overread. GREEN added a release-managed-specific legacy source boundary helper while keeping new sidecar rows on the row-aware decode path.

Focused and related validation passed:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app -run 'TestRuntimeReleaseManagedStatusUsesEncodedErrorText|TestReleaseManagedStatusProjectionIncludesEncodedSourceErrorText' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui -run 'TestRuntimeReleaseManagedStatusLegacyRowKeepsSourceAndMetadata' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'ReleaseManaged|ManagedBundle|RuntimeTabs|RuntimeAction|NewTUIModelForWorkspaceShowsReleaseManaged|RootCommandIncludesProductionCommands' -count=1 -v`

Final repository validation passed after the quality-review fallback fix:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

The spec review subagent found no blocker in emission, parser isolation, sidecar fallback, or regression coverage. The quality review subagent's legacy fallback finding was fixed and reverified. No release smoke was rerun because release packaging and bundled manifests were not touched in this row-format slice.

## Follow-Ups

- Add sidecars for `compile-preview review execution artifact:` `reason` / `error`.
- Add sidecars for `install orchestration artifact:` `error`.
- Add worker identity/path sidecars where parser-consumed values can contain row delimiters.
- Continue broader `message` and remaining display-only text row-value encoding.
