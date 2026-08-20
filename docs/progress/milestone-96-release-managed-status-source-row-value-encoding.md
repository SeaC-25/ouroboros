# Milestone 96: Release Managed Status Source Row-Value Encoding

Milestone 96 extends parser-safe row-value sidecars to Toolchain Health `release managed status source:` rows.

## Scope

Release-managed status-source projections now retain compact human-readable `reason=` and add a RawURL base64 sidecar:

- `reason_b64`

Runtime Tabs now strips legacy `reason` text before parsing source-marker machine metadata, then decodes the sidecar for Inspector/action details. This prevents embedded `status=`, `mode=`, `release_root=`, `total=`, `available=`, `blocked=`, `not_imported=`, `imported=`, `linked=`, `conflict=`, `marker=`, or fake sidecar markers in operator reason text from truncating or retargeting Toolchain Health details.

## Compatibility

The readable `reason=` field remains in each row for existing human-readable output. Legacy rows without sidecars still use the existing fallback path for simple legacy reason text.

This is only a row-format compatibility layer. It does not set, clear, or read new release-managed status sources, import bundles, link managed bundles, install payloads, execute tools, contact providers, dispatch workers, contact endpoints/targets, write reports, or change the existing status-source marker gates.

## Validation

RED parser/projection tests first reproduced the old behavior: source-marker reason text truncated at fake `reason_b64` / machine-looking tokens, machine metadata was retargetable from reason text, and projection rows lacked `reason_b64`. GREEN added sidecar emission, parser isolation through a reason-stripped machine row, sidecar decoding, fake marker coverage, and simple no-sidecar legacy fallback coverage.

Focused and related validation passed before final repository validation:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app -run 'TestRuntimeReleaseManagedStatusSource(UsesEncodedReasonText|LegacyReasonFallback)|TestReleaseManagedStatusSourceProjectionIncludesEncodedReasonText' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'ReleaseManagedToolBundleStatusSource|ReleaseManagedStatusSource|ReleaseManagedStatusRows|ReleaseManaged|ManagedBundle|RuntimeTabs|RuntimeAction|NewTUIModelForWorkspaceShowsReleaseManaged|RootCommandIncludesProductionCommands' -count=1 -v`

Spec and quality review subagents approved the milestone scope with no blocking findings. The residual risk is the already-known generic decoded-field fallback for old rows that contain valid fake map-level `_b64` tokens without a validated adjacent sidecar.

Final repository validation passed after documentation synchronization:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Follow-Ups

- Add sidecars for `managed bundle:` `source`.
- Audit generic decoded-field fallback for legacy rows that contain fake but decodable `_b64` tokens without a validated adjacent sidecar.
- Add worker identity/path sidecars where parser-consumed values can contain row delimiters.
- Continue broader `message` and remaining display-only text row-value encoding.
