# Milestone 21: Tool-Level Catalog Contracts

## Implemented
- Upgraded `toolchain.BuiltInAppSecTools()` from simple catalog metadata into a tool-level contract catalog.
- Each executable built-in tool now declares:
  - Pack name.
  - Manifest dependency name and kind.
  - Target kind.
  - Adapter id.
  - Operation id.
  - Parser id.
  - Category and capabilities.
- Added contract coverage for all 25 executable built-in dependencies:
  - Web/API: `nuclei`, `httpx`, `subfinder`, `ffuf`, `katana`, `gau`, `waybackurls`, `dalfox`, `arjun`, `sqlmap`.
  - Infra/Internal: `nmap`, `naabu`, `dnsx`, `amass`, `dnsrecon`, `testssl.sh`, `sslscan`.
  - APK/mobile/artifact: `apktool`, `jadx`, `mobsfscan`, `semgrep`, `gitleaks`, `trufflehog`, `binwalk`, `exiftool`.
- Kept `nuclei-templates` as a rule-bundle catalog contract tied to `nuclei`.
- Extended the authored adapter/parser registry for previously catalog-only tools:
  - `archive_discovery` for `gau` and `waybackurls`.
  - `xss_scan` for `dalfox`.
  - `parameter_discovery` for `arjun`.
  - `injection_test` for `sqlmap`.
  - Added parser ids such as `url_list_text`, `dalfox_json`, `arjun_json`, `sqlmap_json`, `amass_json`, and `dnsrecon_json`.
- Fixed a manifest/catalog drift by adding `evidence.collection` to `ctf-pack` capabilities in both bundled and initialized workspace manifests.
- Added tests that enforce:
  - Every executable catalog entry references a registered adapter operation and parser.
  - Every root manifest dependency maps to a catalog entry in the same pack with the same dependency kind.
  - Catalog capabilities are declared by the owning pack manifest.
  - Initialized workspace manifests semantically match bundled root manifests.

## Missing
- The compiler still routes pack-level manifests, not individual `BuiltInTool` entries.
- Runtime still does not select a tool id from a multi-dependency pack.
- Registered operations do not yet build argv for managed binaries.
- Registered parsers do not yet parse real outputs into fact/evidence events.
- Real bundled payloads for the declared dependencies are still pending.

## Validation
- RED checks confirmed `BuiltInTool` had no dependency, target, adapter, operation, or parser fields.
- RED checks confirmed `ctf-pack` lacked the catalog-declared `evidence.collection` capability.
- Focused GREEN checks:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain -run 'TestBuiltInAppSecToolsDeclareToolLevelContracts|TestBuiltInAppSecToolsCoverManifestDependencies|TestBuiltInAppSecToolCatalogIncludesCoreTools|TestDefaultAuthoredToolRegistry' -count=1`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestInitWorkspaceCreatesProviderConfigAndToolchainManifests|TestInitWorkspaceToolchainManifestsMatchBundledManifests|TestInspectToolchainsReportsWorkspaceManifestsAndHealth' -count=1`

## Next Step
- Make compiler plans consume `BuiltInTool` contracts:
  - Emit `ToolID`, `AdapterID`, `Operation`, `ParserID`, `TargetKind`, and dependency subset on plan nodes.
  - Select specific tools inside multi-dependency packs.
  - Keep target scope and isolation checks before any argv builder runs.

## Risks And Decisions
- Tool-level catalog contracts are still metadata. They intentionally do not imply the listed binaries are bundled, installed, or executable yet.
- `nuclei-templates` remains a rule-bundle support dependency instead of an executable tool operation.
- Fixing `ctf-pack` capabilities makes artifact/mobile tools visible to evidence collection routing, but parser/runtime implementation remains a separate milestone.
