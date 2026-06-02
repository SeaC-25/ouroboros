# Milestone 22: Tool-Level Compiler Plans

## Implemented
- Upgraded the compiler from pack-level manifest routing to tool-level candidate expansion for built-in catalog tools.
- `compiler.PlanNode` now carries tool execution identity:
  - `ToolID`
  - `AdapterID`
  - `Operation`
  - `ParserID`
  - `TargetKind`
  - `DependencyName`
  - `DependencyKind`
- Built-in multi-dependency packs now emit separate plan nodes for executable tools. The default `discover_assets` workspace plan now expands to:
  - Web/API: `httpx`, `subfinder`
  - Infra/Internal: `amass`, `dnsrecon`, `dnsx`, `naabu`, `nmap`
  - CTF fallback: `ctf-pack` for `ctf.challenge_classification`
- Rule-bundle dependencies are preserved for tool-level operations that need them, including `nuclei` plus `nuclei-templates`.
- Compile preview, compile-review, CLI output, TUI rows, and review identity hashing now include tool-level identity so different tools in the same pack do not collapse into one review node.
- Runtime worker placement, queue claim, dispatch, and transport payloads carry tool-level `tool_plan` fields through to artifacts.
- Worker command environments now expose tool-level fields such as `OURO_TOOL_ID`, `OURO_ADAPTER_ID`, `OURO_OPERATION`, `OURO_PARSER_ID`, `OURO_TARGET_KIND`, `OURO_DEPENDENCY_NAME`, and `OURO_DEPENDENCY_KIND`.
- TUI worker placement sorting now shows ready candidates before skipped candidates, which keeps actionable tool-level placements visible.
- TUI placement execution can batch-claim multiple ready placement candidates after the existing double-confirm gate.

## Missing
- The declared executable tool payloads are still metadata-only unless installed through a managed bundle path.
- Registered operations do not yet build per-tool argv from structured scope inputs.
- Registered parsers do not yet parse real tool output into fact/evidence events.
- Runtime `Tool` top-level fields still preserve legacy manifest names; concrete tool identity is carried in `ToolPlan`.
- CTF/artifact tools still fall back to pack-level planning for `discover_assets` because no executable catalog entry currently owns `ctf.challenge_classification`.

## Validation
- RED checks confirmed runtime worker payloads did not carry tool-level identity and worker command environments did not expose tool-level fields.
- Focused GREEN checks:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/compiler -count=1`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
  - `GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -count=1`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/tui -count=1`
- Full verification:
  - `GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
  - `make doctor-strict`
  - `git diff --check`

## Next Step
- Implement registered argv builders for `managed_binary` operations so model-authored and catalog-selected tools execute through typed adapter operations rather than ad hoc scripts.
- Add parser dispatch for registered parser ids and bind parsed outputs into facts/evidence.
- Add real managed bundle payloads for the highest-priority tools before claiming bundled execution coverage.

## Risks And Decisions
- Tool-level planning intentionally increases candidate counts. Tests and UI now reflect that `discover_assets` is 8 candidates, not 3 packs.
- Safety close paths remain intact: disabled target policy or disabled isolation still require explicit current authorization before side-effect gates run.
- The model-authored-tool path remains structured by registry contracts. The model can author tool specs, but not arbitrary shell scripts.
