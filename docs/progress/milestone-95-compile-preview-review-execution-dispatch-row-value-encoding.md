# Milestone 95: Compile Preview Review Execution Dispatch Row-Value Encoding

Milestone 95 extends parser-safe row-value sidecars to Toolchain Health `compile-preview review execution dispatch artifact:` rows.

## Scope

Compile-preview review execution dispatch projections now retain compact human-readable `reason=` and add a RawURL base64 sidecar:

- `reason_b64`

Runtime Tabs now strips legacy `reason` text before parsing dispatch machine metadata, then decodes the sidecar for Inspector/action details. This prevents embedded `status=`, `mode=`, `action=`, `nodes=`, `warnings=`, `actor=`, `created_at=`, `review=`, `selection=`, `path=`, or fake sidecar markers in dispatch reason text from truncating or retargeting Toolchain Health details.

## Compatibility

The readable `reason=` field remains in each row for existing human-readable output. Legacy rows without sidecars still use the existing fallback path for simple legacy reason text.

This is only a row-format compatibility layer. It does not create compile-preview reviews, select reviews, create dispatches, execute review nodes, contact providers, dispatch workers, contact endpoints/targets, write reports, or change the existing TUI double-confirm dispatch creation/execution gates.

## Validation

RED parser/projection tests first reproduced the old behavior: dispatch reason text truncated at fake `reason_b64` / machine-looking tokens, and projection rows lacked `reason_b64`. GREEN added sidecar emission, parser isolation through a reason-stripped machine row, sidecar decoding, fake marker coverage, and simple no-sidecar legacy fallback coverage.

Focused and related validation passed before final repository validation:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app -run 'TestRuntimeCompilePreviewReviewExecutionDispatchArtifact(UsesEncodedReasonText|LegacyReasonFallback)|TestCompilePreviewReviewExecutionDispatchProjectionIncludesEncodedReasonText' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -run 'CompilePreviewReviewExecutionDispatch|CompilePreviewReview|compile-preview|RuntimeTabs|RuntimeAction|RootCommandIncludesProductionCommands' -count=1 -v`

Spec and quality review subagents approved the milestone scope with no blocking findings. The residual risk is the already-known generic decoded-field fallback for old rows that contain valid fake map-level `_b64` tokens without a validated adjacent sidecar.

Final repository validation passed after documentation synchronization:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Follow-Ups

- Add sidecars for `release managed status source:` operator `reason`.
- Audit generic decoded-field fallback for legacy rows that contain fake but decodable `_b64` tokens without a validated adjacent sidecar.
- Add worker identity/path sidecars where parser-consumed values can contain row delimiters.
- Continue broader `message` and remaining display-only text row-value encoding.
