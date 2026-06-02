# Milestone 20: Typed Authored Tool Registry

## Implemented
- Added a default typed registry for model-authored native tools.
- Registered the `managed_binary` adapter and high-value operations for:
  - HTTP probing, template scanning, path fuzzing, crawling, subdomain discovery, DNS probing, port scanning, service fingerprinting, TLS assessment.
  - APK decompile, static app/code scanning, secret scanning, firmware analysis, and metadata extraction.
- Registered parser ids for existing pack parsers and planned tool output formats, including `httpx_jsonl`, `nuclei_jsonl`, `ffuf_json`, `katana_jsonl`, `subfinder_jsonl`, `dnsx_jsonl`, `nmap_xml`, `naabu_jsonl`, `mobsfscan_json`, and `semgrep_sarif`.
- Authored tool validation now rejects:
  - Unknown `adapter.id`.
  - Unknown `adapter.operation`.
  - Unknown parser ids.
  - Parser ids that are not supported by the selected operation.
  - Missing required operation inputs.
  - Required inputs with incompatible types.
  - Specs that do not declare one of the dependencies required by the selected operation.
- Added `ouro toolchain authored-tool-registry` so users and model/tool authors can inspect allowed adapters, operations, required inputs, parsers, and dependencies.
- Preserved explicit safety close paths:
  - `safety.target_policy: disabled` still requires `allow_target_policy_disabled: true`.
  - `safety.isolation: disabled` still requires `allow_isolation_disabled: true`.

## Missing
- Registry operations are validation contracts only; runtime still does not dispatch by tool id plus adapter operation.
- Real argv builders for `httpx`, `nuclei`, `nmap`, `naabu`, APK tooling, and other dependencies remain pending.
- Parser ids are registered, but real parser dispatch for Nmap XML, JSONL, SARIF, and APK outputs remains pending.
- The compiler still emits pack-level plan nodes, not tool-level plan nodes with `ToolID`, `AdapterID`, `Operation`, and `ParserID`.
- Real release payloads for the listed tools are not yet bundled.
- Authored spec digests are still not carried into generated manifests or execution plans.

## Validation
- RED checks confirmed the registry API and CLI command did not exist before this slice.
- Focused GREEN checks:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain -run 'TestDefaultAuthoredToolRegistry|TestValidateAuthoredToolSpec' -count=1`
  - `GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'TestToolchainAuthoredToolRegistryCommandPrintsContracts|TestToolchainAuthoredToolCommandsValidateWriteListAndRead|TestToolchainAuthoredToolValidateRejects' -count=1`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestCreateAuthoredTool|TestReadAuthoredTool|TestAuthoredToolMetadata' -count=1`
- Full suite:
  - `GOPROXY=https://goproxy.cn,direct go test ./... -count=1`

## Next Step
- Make the registry the source of truth for tool-level execution planning:
  - Add `ToolID`, `AdapterID`, `Operation`, and `ParserID` to compiler/run plans.
  - Select specific dependencies inside multi-tool packs instead of falling back to pack-level native execution.
  - Add registered argv builders that only consume validated input bindings.
  - Dispatch registered parsers into evidence/fact events.

## Risks And Decisions
- The registry deliberately blocks model-generated scripts and command arrays. Models can author tools by selecting a registered operation and inputs, not by producing shell snippets.
- Parser ids were registered before all parsers exist so authored specs can be constrained now; runtime parser implementation must not claim support until fixture-backed parsers are added.
- Dependencies are required at the operation-contract level, but installation and release payload packaging remain explicit separate steps.
