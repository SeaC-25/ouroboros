# Milestone 91: Toolchain Execution Result Row-Value Encoding

Milestone 91 extends parser-safe row-value sidecars to Toolchain Health install execution result rows that display command output and retry failure text in the TUI Inspector.

## Scope

Workspace-bound Toolchain Health projections now keep the legacy readable fields and add RawURL base64 sidecars for:

- `execution result <pack>/<dependency>:` `output_b64` and `command_b64`
- `retry execution result <pack>/<dependency>:` `output_b64`, `error_b64`, and `command_b64`
- `retry execution result refs <pack>/<dependency>:` `command_b64`

Runtime Tabs now strips legacy `output`, `error`, and `command` text before reading machine metadata for those rows and decodes the sidecars for Inspector text. The sidecar matcher now requires decoded text to match the adjacent readable field before using it as a row boundary, so later fake sidecar tokens in other readable fields cannot override the generated sidecar. This prevents embedded `artifact=`, `dispatch=`, `retry_dispatch=`, `source_execution=`, `output_b64=`, `error=`, or `command=` tokens in command output, retry error text, or command text from retargeting install execution metadata.

`retry execution result refs summary` rows now parse as a dedicated read-only summary action instead of falling through to the ordinary retry execution result parser.

## Compatibility

The readable `output=`, `error=`, and `command=` fields remain in each row for existing human-readable output. Legacy rows without `command_b64` still fall back to command parsing from the output/error-stripped machine row.

This is only a row-format compatibility layer. It does not add install execution shortcuts, retry execution shortcuts, package-manager calls, target contact, provider calls, replay mutation, or artifact writes beyond the pre-existing Toolchain Health projection rows.

## Validation

RED parser tests first reproduced output/error truncation and metadata retargeting for install execution result and retry execution result rows. RED projection tests also proved the rows lacked `output_b64` and `error_b64` sidecars. Review-driven RED tests then covered naked `command` pollution, later fake sidecar override, Inspector rendering of decoded text, and `retry execution result refs summary` misclassification. GREEN added sidecar emission, parser isolation, row-aware sidecar matching, `command_b64`, and a dedicated refs summary action.

Focused and related validation passed:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app -run 'TestRuntimeInstall(ExecutionResultUsesEncodedOutputText|ExecutionResultUsesEncodedCommandText|RetryExecutionResultUsesEncodedOutputErrorText|RetryExecutionResultIgnoresLaterFakeOutputSidecar|RetryExecutionResultUsesEncodedCommandText)|TestRuntimeRetryExecutionResultRefsSummaryUsesSummaryAction|TestModelUpdateShowsSelectedInstall(ExecutionResultDetailsDecodeEncodedOutput|RetryExecutionResultDetailsDecodeEncodedOutputError)|TestToolchain(InstallExecutionResultProjectionIncludesEncodedOutputText|RetryExecutionResultProjectionIncludesEncodedOutputErrorText)' -count=1 -v`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui -run 'InstallExecution|InstallRetry|RetryExecution|Toolchain|RuntimeTabs|RuntimeAction|NewTUIModelForWorkspaceShowsInstall|ModelUpdateShowsSelectedInstall|UsesEncoded|IgnoresFake|RowValue|ReplayWorkbench|ReportProjection|WorkerLifecycle' -count=1 -v`

Final repository validation passed:

- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build`
- `sh -n scripts/release.sh`
- `git diff --check`
- `gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go`

## Follow-Ups

- Add sidecars for `release managed status:` `error`.
- Add sidecars for `compile-preview review execution artifact:` `reason` / `error`.
- Add sidecars for `install orchestration artifact:` `error`.
- Continue broader `message` and remaining display-only text row-value encoding.
