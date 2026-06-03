# Milestone 23: Managed Binary Argv Builders

## Implemented
- Added the first registered native `managed_binary` argv builder path for tool-level catalog nodes.
- `nativeManagedRunCommand` now consults structured `PlanNode` identity for `adapter_id`, `operation`, `dependency_name`, and `dependency_kind` before building argv.
- Added typed process-mode argv builders for:
  - `httpx` / `http_probe`: `httpx -json -u <target>`
  - `nmap` / `service_fingerprint`: `nmap -oX - <target>`
- Kept execution routed through the existing command runner, so workspace-managed binary resolution, scope target checks, exclusion checks, argument validation, attempt directories, timeout handling, and process/container isolation semantics remain centralized.
- Native command environments now expose tool-level identity with `OURO_TOOL_ID`, `OURO_ADAPTER_ID`, `OURO_OPERATION`, `OURO_PARSER_ID`, `OURO_TARGET_KIND`, `OURO_SCOPE_TARGET_KIND`, `OURO_DEPENDENCY_NAME`, and `OURO_DEPENDENCY_KIND`.
- Preserved fail-closed behavior:
  - Unknown adapter operations stay offline.
  - Unsupported tool/operation pairs stay offline.
  - Missing scope target context stays offline.
  - Missing binary dependency contracts stay offline.
  - Missing workspace-managed binaries stay offline instead of falling through to ambient `PATH`.
  - Supported builders hand the command runner the resolved workspace-managed payload path, not a bare binary name.
  - Container isolation still keeps native managed argv builders offline.

## Not Implemented
- Most registered `managed_binary` operations still need argv builders, including nuclei, ffuf, katana, subfinder, amass, dnsx, dnsrecon, naabu, TLS tools, APK/static/secret scanners, firmware tools, and metadata extractors.
- Runtime parser dispatch still does not parse real tool stdout/stderr into facts or evidence events.
- Real production payloads are still not bundled into the release; tests use managed fixture binaries.
- Domain/host target derivation for tools whose catalog `target_kind` is more specific than the workspace scope target model is still incomplete.
- Container-mode managed execution remains offline for this path.

## Validation
- RED first:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRunWorkspace(ExecutesToolLevelCatalogNodeThroughRegisteredArgvBuilder|ToolLevelManagedBinaryRejectsExcludedScopeBeforeExecution)' -count=1 -v`
  - `TestRunWorkspaceExecutesToolLevelCatalogNodeThroughRegisteredArgvBuilder` failed because the installed `httpx` fixture did not execute.
  - `TestRunWorkspaceToolLevelManagedBinaryRejectsExcludedScopeBeforeExecution` failed because the tool-level node stayed offline and did not hit the exclusion failure.
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRunWorkspaceExecutesToolLevelNmapThroughRegisteredArgvBuilder' -count=1 -v`
  - `TestRunWorkspaceExecutesToolLevelNmapThroughRegisteredArgvBuilder` failed because the installed `nmap` fixture did not execute.
- Focused GREEN:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRunWorkspace(ExecutesToolLevelCatalogNodeThroughRegisteredArgvBuilder|ToolLevelManagedBinaryRejectsExcludedScopeBeforeExecution|ExecutesToolLevelNmapThroughRegisteredArgvBuilder)|TestNativeManagedRunCommand(KeepsUnknownToolLevelOperationOffline|RequiresWorkspaceManagedBinaryForToolLevelBuilder|UsesResolvedWorkspaceManagedBinaryForToolLevelBuilder|KeepsToolLevelBuilderOfflineInContainerIsolation|RequiresBinaryDependencyForToolLevelBuilder)' -count=1 -v`
- Broader verification:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
  - `GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
  - `make doctor-strict`
  - `git diff --check`

## Next Step
- Continue the argv builder registry across the remaining high-value catalog tools, starting with nuclei plus nuclei-templates, subfinder/domain discovery, DNS probes, and naabu.
- Then wire registered parser dispatch so tool output can create facts/evidence instead of staying as transcripts only.

## Risks / Decisions
- The implementation deliberately keeps builder output as argv only, not arbitrary scripts.
- The runner requires a binary dependency contract, then verifies and resolves the first argv element as a workspace-managed binary before handing the command to the command runner, preventing a tool-level builder from using ambient system `PATH`.
- The initial scope model still supports `url`, `cidr`, `challenge`, and `artifact`; domain/host-specific tool inputs need a dedicated derivation layer before all catalog tools can execute safely.
