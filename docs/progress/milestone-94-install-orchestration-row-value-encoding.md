# Milestone 94: Install Orchestration Row-Value Encoding

Milestone 94 extends parser-safe row-value sidecars to Toolchain Health `install orchestration artifact:` rows.

## Scope

Install orchestration projections now retain compact human-readable `error=` and add a RawURL base64 sidecar:

- `error_b64`

Runtime Tabs now strips legacy `error` text before parsing orchestration machine metadata, then decodes the sidecar for Inspector/action details. This prevents embedded `status=`, `mode=`, `ready=`, `skipped=`, `results=`, `dispatch=`, `execution=`, `retry_dispatch=`, or fake sidecar markers in orchestration error text from truncating or retargeting read-only Toolchain Health details.

## Compatibility

The readable `error=` field remains in each row for existing human-readable output. Legacy rows without sidecars still use the existing fallback path for simple legacy error text.

This is only a row-format compatibility layer. It does not create install requests, approve requests, create dispatches, execute install commands, execute retry commands, add an install orchestration TUI execution shortcut, contact endpoints/targets, dispatch workers, or change existing explicit execution gates.

## Validation

RED parser/projection tests first reproduced the old behavior by temporarily restoring the pre-sidecar emitter/parser: the parser truncated orchestration error text, and projection rows lacked `error_b64`. GREEN restored sidecar emission, parser isolation through an error-stripped machine row, sidecar decoding, fake marker coverage, and simple no-sidecar legacy fallback coverage.

Focused and related validation passed before final repository validation:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app -run 'TestRuntimeInstallOrchestrationArtifact(UsesEncodedErrorText|LegacyErrorFallback)|TestToolchainInstallOrchestrationProjectionIncludesEncodedErrorText' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'InstallOrchestration|install-orchestrat|ToolchainInstall|RuntimeTabs|RuntimeAction|NewTUIModelForWorkspaceShowsInstall|ModelUpdateShowsSelectedInstall|RootCommandIncludesProductionCommands' -count=1 -v`

Spec and quality review subagents approved the milestone scope with no blocking findings. The residual risk is the already-known generic decoded-field fallback for old rows that contain valid fake map-level `_b64` tokens without a validated adjacent sidecar.

Final repository validation passed after documentation synchronization:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Follow-Ups

- Add sidecars for `compile-preview review execution dispatch artifact:` `reason`.
- Add sidecars for `release managed status source:` operator `reason`.
- Audit generic decoded-field fallback for legacy rows that contain fake but decodable `_b64` tokens without a validated adjacent sidecar.
- Add worker identity/path sidecars where parser-consumed values can contain row delimiters.
- Continue broader `message` and remaining display-only text row-value encoding.
