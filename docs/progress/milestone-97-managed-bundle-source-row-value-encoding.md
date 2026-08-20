# Milestone 97: Managed Bundle Source Row-Value Encoding

Milestone 97 extends parser-safe row-value sidecars to workspace-local Toolchain Health `managed bundle:` rows.

## Scope

Managed bundle projections now retain compact human-readable `source=` and add a RawURL base64 sidecar:

- `source_b64`

Runtime Tabs now strips legacy `source` text before parsing managed-bundle machine metadata, then decodes the sidecar for Inspector/action details. This prevents embedded `status=`, `mode=`, `name=`, `sha256=`, `payload=`, `install=`, `manifest=`, or fake sidecar markers in manifest/operator source text from truncating or retargeting Toolchain Health details.

## Compatibility

The readable `source=` field remains in each row for existing human-readable output. Legacy rows without sidecars still use the existing fallback path for simple single-token source text.

This is only a row-format compatibility layer. It does not import bundles, link bundles, install payloads, execute tools, probe health, contact providers, dispatch workers, contact endpoints/targets, write reports, or change managed-bundle import/install gates.

## Validation

RED parser/projection tests first reproduced the old behavior: managed-bundle source text truncated at fake `source_b64` / machine-looking tokens, machine metadata was retargetable from source text, and projection rows lacked `source_b64`. GREEN added sidecar emission, parser isolation through a source-stripped machine row, sidecar decoding, fake marker coverage, and retained simple no-sidecar legacy fallback behavior.

Focused and related validation passed before final repository validation:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app -run 'TestRuntimeManagedBundleUsesEncodedSourceText|TestManagedBundleProjectionIncludesEncodedSourceText|TestNewTUIModelForWorkspaceShowsManagedBundleRowsReadOnly' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'ManagedBundle|ReleaseManaged|RuntimeTabs|RuntimeAction|NewTUIModelForWorkspaceShowsManagedBundle|NewTUIModelForWorkspaceShowsReleaseManaged|RootCommandIncludesProductionCommands' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`

Spec review approved the milestone scope with no blocking findings. The first quality review subagent disconnected before completion, and the narrower replacement quality review timed out without producing findings; local diff review plus focused/related/full validation carried the quality gate. The residual risk is the already-known generic decoded-field fallback for old rows that contain valid fake map-level `_b64` tokens without a validated adjacent sidecar.

Final repository validation passed after documentation synchronization:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Follow-Ups

- Audit generic decoded-field fallback for legacy rows that contain fake but decodable `_b64` tokens without a validated adjacent sidecar.
- Add worker identity/path sidecars where parser-consumed values can contain row delimiters.
- Continue broader `message` and remaining display-only text row-value encoding.
