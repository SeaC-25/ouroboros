# Milestone 93: Compile Preview Review Execution Row-Value Encoding

Milestone 93 extends parser-safe row-value sidecars to Toolchain Health `compile-preview review execution artifact:` rows.

## Scope

Compile-preview review execution projections now retain compact human-readable fields and add RawURL base64 sidecars for:

- `reason_b64`
- `error_b64`

Runtime Tabs now strips legacy `reason` and `error` text before parsing execution machine metadata, then decodes the sidecars for Inspector/action details. This prevents embedded `run=`, `dispatch=`, `review=`, `selection=`, `path=`, `events=`, `actor=`, `executed_at=`, `status=`, `mode=`, `action=`, `nodes=`, `warnings=`, or fake sidecar markers in execution reason/error text from truncating or retargeting read-only Toolchain Health details.

## Compatibility

The readable `reason=` and `error=` fields remain in each row for existing human-readable output. Legacy rows without sidecars still use the existing fallback path for simple legacy reason/error text.

This is only a row-format compatibility layer. It does not execute compile-preview reviews, create dispatches, select reviews, write reports, call providers, dispatch workers, contact endpoints/targets, or change the existing explicit execution gates.

## Validation

RED parser tests first reproduced reason/error truncation and fake sidecar pollution in `compile-preview review execution artifact:` rows. RED projection tests also proved the rows lacked `reason_b64` and `error_b64`. GREEN added sidecar emission, parser isolation through reason/error-stripped machine rows, sidecar decoding, fake marker coverage, and no-sidecar legacy fallback coverage.

Focused and related validation passed before final repository validation:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app -run 'TestRuntimeCompilePreviewReviewExecutionArtifact(UsesEncodedReasonErrorText|LegacyReasonErrorFallback)|TestCompilePreviewReviewExecutionProjectionIncludesEncodedReasonErrorText' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'CompilePreview|compile-preview|RuntimeTabs|RuntimeAction|RootCommandIncludesProductionCommands' -count=1 -v`

Final repository validation passed after documentation synchronization and read-only review:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

The read-only quality review found no blocker. Its low-priority note is that the generic decoded-field fallback can still decode map-level fake `_b64` tokens on old rows that have no validated adjacent sidecar; that is recorded as a broader follow-up rather than expanding this milestone.

## Follow-Ups

- Add sidecars for `compile-preview review execution dispatch artifact:` `reason`.
- Add sidecars for `install orchestration artifact:` `error`.
- Audit generic decoded-field fallback for legacy rows that contain fake but decodable `_b64` tokens without a validated adjacent sidecar.
- Add worker identity/path sidecars where parser-consumed values can contain row delimiters.
- Continue broader `message` and remaining display-only text row-value encoding.
