# Milestone 18: Native Managed Runner

## Implemented
- Extended compiler plan nodes with manifest dependency metadata so runtime can inspect the selected manifest's dependency shape.
- Added native managed runner dispatch for `runner: native` manifests:
  - first resolves `workspace/toolchain/bin/<manifest-name>`;
  - if absent, resolves the manifest's only binary dependency when exactly one is declared;
  - falls back to the offline boundary when no managed binary is available.
- Native managed dispatch builds a direct argv, appends the scoped target when present, and delegates to the existing command runner instead of adding a second direct execution path.
- Command runner executable resolution now prefers workspace-local managed binaries before ambient `PATH`.
- Workspace binary symlinks are resolved and must point back under `workspace/toolchain/managed-bundles`; symlinks outside the managed bundle root are rejected.
- Command environments now include:
  - `OURO_TOOLCHAIN_BIN=<workspace>/toolchain/bin`
  - `OURO_TOOLCHAIN_RULES=<workspace>/toolchain/rules`
  - `PATH=<workspace>/toolchain/bin:$PATH`
- Added tests proving a process-mode native manifest can execute an installed managed `httpx` payload and records the result in durable tool events.
- Added tests proving an excluded scoped target blocks before the native managed payload executes.

## Missing
- Real release payloads for `nmap`, `nuclei`, `httpx`, `subfinder`, `ffuf`, `katana`, `naabu`, `dnsx`, `apktool`, `jadx`, and related tools are still not bundled.
- Native managed execution currently avoids container mode because workspace symlink targets and in-container tool availability need a mount-safe design.
- Tool-specific argv generation is still generic. A typed native adapter registry must replace the current "manifest name or single binary dependency plus scoped target" fallback before broad real-world execution.
- Model-authored native tools still need adapter IDs, operation IDs, spec digests, parser registry validation, and dependency declarations that cannot smuggle command arrays.
- Parser bindings for Nmap XML, nuclei JSONL, httpx JSONL, and related outputs remain pending.

## Validation
- RED app checks confirmed installed managed binaries were not executed by `runner: native`.
- RED app checks confirmed exclusion handling did not yet prove payload non-execution for native managed tools.
- Focused GREEN checks:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRunWorkspaceExecutesNativeManagedBundleFromWorkspaceToolchainBin|TestRunWorkspaceNativeManagedBundleRejectsExcludedScopeBeforeExecution' -count=1`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/compiler ./internal/app -count=1`

## Next Step
- Tighten model-authored tool dependencies so authored specs cannot carry `health_command` or `install_command`, then introduce typed native adapter and parser registries.
- Add release payload build assets for the first high-value tools.
- Add parser bindings and evidence projection for real managed tool outputs.

## Risks And Decisions
- This slice deliberately reuses the existing command runner so target-policy, exclusion, timeout, attempt-directory, isolation, and strict run authorization behavior are not forked.
- Native managed execution only runs when a workspace-managed binary is already installed. It does not fetch tools, run installers, or infer one binary from multi-dependency packs.
- Rule bundle paths are exposed through environment, not PATH. Tool-specific adapters must decide how to pass templates/rules to individual tools.
