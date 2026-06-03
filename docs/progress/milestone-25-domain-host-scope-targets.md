# Milestone 25: Domain And Host Scope Targets

## Implemented
- Added explicit scope target kinds:
  - `domain`
  - `host`
- Mapped `domain` and `host` targets to the Infra/Internal domain pack for hypothesis seeding.
- Updated Infra/Internal initial hypotheses to accept CIDR, domain, and host targets while still validating CIDR syntax for CIDR scope.
- Added execution-target resolution that lets tool-level catalog nodes use `PlanNode.TargetKind` instead of only capability prefixes.
- Preserved host/CIDR compatibility for existing `target_kind=host` infra tools, so prior CIDR `nmap` and `naabu` flows still work.
- Added `subfinder` / `subdomain_discovery` argv construction only for explicit domain scope:
  - `subfinder -json -d <domain>`
- Preserved URL-scope safety:
  - URL targets do not imply domain or wildcard-subdomain authorization.
  - `subfinder`, `amass`, `dnsx`, and `dnsrecon` still remain offline for URL-only scope.
- Preserved exclusion enforcement for explicit domain scope before payload execution.
- Added host-scope nmap execution:
  - `nmap -oX - <host>`

## Not Implemented
- `amass`, `dnsx`, and `dnsrecon` argv builders remain offline until their parser/file-output boundaries are implemented.
- Domain/host command-literal validation for arbitrary command manifests remains follow-up work; managed builders use structured target selection and exclusion checks.
- Parser dispatch still does not convert real subfinder/nmap output into evidence graph facts.
- Real managed payload packaging is still separate from this scope-model slice.

## Validation
- RED first:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestValidateScopeConfigAcceptsDomainAndHostTargets|TestRunWorkspaceExecutesToolLevelSubfinderThroughExplicitDomainScope' -count=1 -v`
  - `TestValidateScopeConfigAcceptsDomainAndHostTargets` failed with `target 0 kind is unsupported: domain`.
  - `TestRunWorkspaceExecutesToolLevelSubfinderThroughExplicitDomainScope` failed because `scope.yaml` was invalid for `kind: domain`.
- Focused GREEN:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestValidateScopeConfigAcceptsDomainAndHostTargets|TestRunWorkspaceExecutesToolLevelSubfinderThroughExplicitDomainScope|TestRunWorkspaceRejectsExcludedDomainBeforeSubfinderExecution|TestRunWorkspaceExecutesToolLevelNmapThroughExplicitHostScope|TestNativeManagedRunCommandKeepsSubfinderOfflineForURLScope|TestNativeManagedRunCommandKeepsDomainToolsOfflineForURLScope|TestRunWorkspaceExecutesToolLevelNmapThroughRegisteredArgvBuilder|TestRunWorkspaceExecutesToolLevelNaabuThroughRegisteredArgvBuilder' -count=1 -v`
- Broader app verification:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
- Review fixes:
  - Added coverage that `naabu` stays offline for explicit `host` scope and still chooses CIDR when both `host` and `cidr` targets exist.
  - Added coverage that malformed/future `nmap` tool-level nodes with unsupported target kinds stay offline.
  - Added full-run coverage that an excluded CIDR blocks `naabu` execution even when a non-excluded host target is also present.
  - Added coverage that `amass`, `dnsx`, and `dnsrecon` remain offline even with explicit `domain` scope until their own builders are implemented.
  - Updated managed-binary target selection to be operation/binary-aware: `naabu` selects CIDR only, `nmap` selects host/CIDR only, `subfinder` selects domain only, and `nuclei`/`httpx` select URL only.
  - Passed the managed-binary selected scope target through the command runner so exclusion checks, argument validation, environment, and template expansion use the same target as the generated argv.
- Review/focused verification:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNativeManagedRunCommand(BuildsNucleiTemplateScanArgvWithRules|KeepsNucleiOfflineWithoutRuleBundle|RejectsEscapingNucleiRuleBundleSymlink|KeepsNucleiOfflineWithoutRuleBundleDependencyContract|KeepsNucleiOfflineForNonURLScope|KeepsNaabuOfflineForNonCIDRScope|UsesCIDRForNaabuWhenHostAndCIDRScopeExist|KeepsSubfinderOfflineForURLScope|KeepsDomainToolsOfflineForURLScope|KeepsUnsupportedDomainToolsOfflineForDomainScope|KeepsNmapOfflineForUnsupportedTargetKind|KeepsUnknownToolLevelOperationOffline)|TestRunWorkspaceExecutesToolLevel(NaabuThroughRegisteredArgvBuilder|SubfinderThroughExplicitDomainScope|NmapThroughExplicitHostScope)|TestRunWorkspaceRejectsExcludedDomainBeforeSubfinderExecution' -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNativeManagedRunCommand(BuildsNucleiTemplateScanArgvWithRules|KeepsNucleiOfflineWithoutRuleBundle|RejectsEscapingNucleiRuleBundleSymlink|KeepsNucleiOfflineWithoutRuleBundleDependencyContract|KeepsNucleiOfflineForNonURLScope|KeepsNaabuOfflineForNonCIDRScope|UsesCIDRForNaabuWhenHostAndCIDRScopeExist|KeepsSubfinderOfflineForURLScope|KeepsDomainToolsOfflineForURLScope|KeepsUnsupportedDomainToolsOfflineForDomainScope|KeepsNmapOfflineForUnsupportedTargetKind|KeepsUnknownToolLevelOperationOffline)|TestRunWorkspaceExecutesToolLevel(NaabuThroughRegisteredArgvBuilder|SubfinderThroughExplicitDomainScope|NmapThroughExplicitHostScope)|TestRunWorkspaceRejectsExcluded(DomainBeforeSubfinderExecution|CIDRBeforeNaabuExecutionWhenHostScopePresent)' -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/domain/infra -count=1`
  - `git diff --check`
- Full verification after review fixes:
  - `GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
  - `make doctor-strict`
  - `git diff --check`

## Next Step
- Add safe argv builders and output handling for `dnsx` and `dnsrecon`.
- Add parser dispatch for subfinder JSONL and DNS outputs.
- Extend explicit host/domain literal checks for command-bound manifests.

## Risks / Decisions
- Explicit `domain` is a separate authorization boundary. A scoped URL such as `https://app.example` still does not authorize `subfinder -d app.example`.
- `host` scope is explicit and can drive nmap service fingerprinting, while `naabu` remains CIDR-only in this slice.
- Domain exclusions use the existing exact/wildcard string exclusion path before command execution.
