# Milestone 14: AppSec Tool Bundle And Native Authoring

## Implemented
- Added first-class appsec tool catalog metadata through `toolchain.BuiltInAppSecTools`.
- Built-in catalog now includes core Web/API tools: `nuclei`, `nuclei-templates`, `httpx`, `subfinder`, `ffuf`, `katana`, `gau`, `waybackurls`, `dalfox`, `arjun`, and `sqlmap`.
- Built-in catalog now includes Infra/Internal tools: `nmap`, `naabu`, `dnsx`, `amass`, `dnsrecon`, `testssl.sh`, and `sslscan`.
- Built-in catalog now includes artifact/app analysis tools under the CTF/Lab pack: `apktool`, `jadx`, `mobsfscan`, `semgrep`, `gitleaks`, `trufflehog`, `binwalk`, and `exiftool`.
- Updated `toolchains/*/manifest.yaml` so the bundled packs declare those tools as visible dependencies with kind, source, health command, install hint, and deterministic install command metadata.
- Added `rule_bundle` dependency kind for `nuclei-templates`, with health reporting through the normal dependency runner instead of pretending template bundles are binaries.
- Updated `InitWorkspace` embedded manifest templates so newly initialized workspaces receive the same appsec dependency metadata as the bundled manifests.
- Added model-authored native tool spec validation through `AuthoredToolSpec`, `ValidateAuthoredToolSpec`, and `AuthoredToolToManifest`.
- Authored tools must be structured native specs with declared capabilities, parser, inputs, expected evidence, and safety settings.
- Authored tools reject `raw_script`, `run_command`, and non-`native` runners so model output cannot silently become an ad hoc shell script.
- Authored tool safety metadata keeps explicit close paths for `target_policy: disabled` and `isolation: disabled` through opt-in fields without changing workspace scope or authorization state.
- Install dispatch and install retry-dispatch execution now require current safety-bypass authorization when `scope.target_policy` or `execution.isolation` is disabled. Without current `authorization.yaml` metadata, installer commands do not run and execution audit artifacts are not written.
- Optional binary and rule-bundle dependencies use lightweight presence checks instead of executing third-party health commands during default run/toolchain health.

## Missing
- Third-party binaries and nuclei template/rule bundles are not yet vendored into the release artifact.
- The `ouro-toolchain install ...` command referenced by install metadata is still a controlled installer contract, not a completed bundled installer implementation.
- Authored native tool specs can be validated and converted to manifests; workspace write/list/read is tracked in Milestone 15. A true native executor for model-authored tools is still missing.
- Parser bindings for real tool output formats such as Nmap XML and JSONL from `nuclei`/`httpx` are still thin.
- Worker `--use-request-command` execution still needs the same target-boundary and isolation guarantees as the local command runner before it can safely carry appsec command payloads.

## Validation
- Baseline before changes: `make test`.
- RED checks confirmed missing dependency metadata in `internal/toolchain` and `internal/app`.
- RED checks confirmed authored native tool APIs were absent.
- Focused GREEN checks:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain -run 'TestBuiltInAppSecTool|TestAuthoredTool|TestValidateAuthoredTool'`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestInitWorkspaceCreatesProviderConfigAndToolchainManifests|TestInspectToolchainsReportsWorkspaceManifestsAndHealth'`
- Regression found and fixed: optional dependency health commands initially executed third-party binaries during run bootstrap, delaying interactive/pause tests. Added `TestCheckDependenciesDoesNotExecuteOptionalHealthCommand` and changed optional dependency checks to lightweight presence/provenance checks.
- Code review found and fixed: install dispatch/retry execution used warning-only doctor checks under disabled boundaries. Added strict authorization tests so missing authorization blocks installer command execution, while authorized disabled boundaries still execute and persist warnings.
- Post-fix regression checks:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain -run TestCheckDependenciesDoesNotExecuteOptionalHealthCommand -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain -run 'TestCheckDependenciesDoesNotExecuteOptional|TestBuiltInAppSecToolDependenciesAreDeclared' -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestRunWorkspacePauseControllerDelaysWorkerStartUntilResume -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestExecuteToolchainInstallDispatch.*Safety|TestExecuteToolchainInstallRetryDispatch.*Safety|TestExecuteToolchainInstallDispatchRunsReadyCommandsAndWritesAuditArtifact|TestExecuteToolchainInstallRetryDispatchRunsReadyCommandsAndWritesAuditArtifact' -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'TestToolchainExecuteInstallDispatchCommand.*Safety|TestToolchainExecuteInstallDispatchCommandRunsWithExecuteFlag' -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run TestRunCommandInteractiveCancelControlCancelsRunContext -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain ./internal/app ./cmd/ouro`

## Next Step
- Integrate persisted authored native manifests into the runtime compiler/executor path once native execution contracts, parser bindings, and safety gates are ready.
- Implement the managed installer/bundle layer for declared tools without using target-contacting install commands; keep installer command shapes allowlisted.
- Extend parser bindings and runtime target-kind checks before any new native appsec executor contacts scoped targets.

## Risks And Decisions
- Tool dependencies are optional in this slice so fresh workspaces remain usable even when the host lacks the third-party tools. This avoids blocking `doctor` and TUI health while the binary bundle is still incomplete.
- New target-contacting tools should reuse existing `web.*` and `infra.*` capabilities until the compiler and command runner explicitly support another target-kind mapping.
- `scope.target_policy: disabled` and `execution.isolation: disabled` remain closeable safety paths; strict authorization gates continue to own whether those disabled modes may execute.
