# Milestone 24: Expanded Managed Binary Argv Builders

## Implemented
- Extended the tool-level `managed_binary` argv builder registry beyond the first `httpx` and `nmap` slice.
- Added `nuclei` / `template_scan` argv construction for URL scope:
  - `nuclei -jsonl -silent -no-color -disable-update-check -no-stdin -ni -t <workspace>/toolchain/rules/nuclei-templates -u <url>`
- Required both a workspace-managed `nuclei` binary and a workspace-installed `nuclei-templates` rule bundle before `nuclei` can leave the offline boundary.
- Added workspace rule-bundle resolution for `workspace/toolchain/rules/<name>`, including symlink resolution checks that keep linked rule bundles under `workspace/toolchain/managed-bundles`.
- Added `naabu` / `port_scan` argv construction for CIDR scope:
  - `naabu -json -host <cidr>`
- Preserved workspace-managed binary resolution: builders still return bare tool names internally and `nativeManagedRunCommand` replaces `argv[0]` with the resolved managed payload path before execution.
- Kept `subfinder` offline under URL-only scope. Current URL authorization is not treated as wildcard domain or subdomain enumeration authorization.

## Not Implemented
- Explicit `domain` / `host` scope support is still pending, so `subfinder`, `amass`, `dnsx`, `dnsrecon`, TLS host tools, and URL-to-host port scanning remain constrained.
- Parser dispatch still does not convert real nuclei/naabu output into facts or evidence graph nodes.
- Production binary and rule-bundle payloads still need release packaging; tests use managed fixture payloads.
- Container-mode managed execution remains offline for this path.

## Validation
- RED first:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNativeManagedRunCommandBuildsNucleiTemplateScanArgvWithRules|TestRunWorkspaceExecutesToolLevelNaabuThroughRegisteredArgvBuilder|TestNativeManagedRunCommandKeepsSubfinderOfflineForURLScope' -count=1 -v`
  - `TestNativeManagedRunCommandBuildsNucleiTemplateScanArgvWithRules` failed with `ok=false`.
  - `TestRunWorkspaceExecutesToolLevelNaabuThroughRegisteredArgvBuilder` failed because the naabu fixture marker was not created.
  - The subfinder URL-scope boundary test passed before implementation and remained a guardrail.
- Focused GREEN:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNativeManagedRunCommandBuildsNucleiTemplateScanArgvWithRules|TestRunWorkspaceExecutesToolLevelNaabuThroughRegisteredArgvBuilder|TestNativeManagedRunCommandKeepsSubfinderOfflineForURLScope' -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNativeManagedRunCommand(BuildsNucleiTemplateScanArgvWithRules|KeepsNucleiOfflineWithoutRuleBundle|RejectsEscapingNucleiRuleBundleSymlink|KeepsSubfinderOfflineForURLScope)|TestRunWorkspaceExecutesToolLevelNaabuThroughRegisteredArgvBuilder' -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNativeManagedRunCommand(KeepsNucleiOfflineWithoutRuleBundleDependencyContract|KeepsNucleiOfflineForNonURLScope|KeepsNaabuOfflineForNonCIDRScope|KeepsDomainToolsOfflineForURLScope|KeepsSubfinderOfflineForURLScope|KeepsNucleiOfflineWithoutRuleBundle|RejectsEscapingNucleiRuleBundleSymlink)' -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRunWorkspace(ExecutesToolLevelCatalogNodeThroughRegisteredArgvBuilder|ExecutesToolLevelNmapThroughRegisteredArgvBuilder|ExecutesToolLevelNaabuThroughRegisteredArgvBuilder|ToolLevelManagedBinaryRejectsExcludedScopeBeforeExecution)|TestNativeManagedRunCommand(BuildsNucleiTemplateScanArgvWithRules|KeepsNucleiOfflineWithoutRuleBundle|RejectsEscapingNucleiRuleBundleSymlink|KeepsNucleiOfflineWithoutRuleBundleDependencyContract|KeepsNucleiOfflineForNonURLScope|KeepsNaabuOfflineForNonCIDRScope|KeepsDomainToolsOfflineForURLScope|KeepsSubfinderOfflineForURLScope|KeepsUnknownToolLevelOperationOffline|RequiresWorkspaceManagedBinaryForToolLevelBuilder|UsesResolvedWorkspaceManagedBinaryForToolLevelBuilder|KeepsToolLevelBuilderOfflineInContainerIsolation|RequiresBinaryDependencyForToolLevelBuilder)' -count=1 -v`
- Broader app verification:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
- Full verification:
  - `GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
  - `make doctor-strict`
  - `git diff --check`

## Next Step
- Add explicit domain/host target modeling before enabling subfinder, DNS probes, TLS assessment, or URL-to-host port scanning.
- Add parser dispatch for nuclei JSONL and naabu JSONL so outputs become evidence/facts instead of transcript-only records.
- Continue release managed payload packaging for binaries and rule bundles.

## Risks / Decisions
- URL scope is not a root-domain or wildcard-subdomain grant. This keeps subdomain enumeration offline until the scope model can represent explicit domain/host authorization.
- `nuclei` requires the managed `nuclei-templates` rule bundle; otherwise the builder stays offline instead of silently scanning with ambient templates.
- `naabu` is limited to CIDR scope in this slice, matching the existing workspace scope model and target-policy validator.
- Code review requested additional negative coverage for domain-tool URL scope, missing nuclei rule-bundle contracts, non-URL nuclei scope, and non-CIDR naabu scope; those guardrails are now covered.
