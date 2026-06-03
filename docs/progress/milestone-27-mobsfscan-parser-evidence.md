# Milestone 27: Mobsfscan Parser Evidence

## Implemented
- Added a narrow run-side parser dispatcher for completed tool executions with `parser_id: mobsfscan_json`.
- Parse success emits one `EventParser` with `status: parsed` and one `EventFact` per MobSF rule finding.
- Parser facts use `kind: finding` and `source: parser.mobsfscan_json`, so replay snapshot projection turns them into blackboard findings and Evidence Graph finding nodes.
- Captured structured finding metadata in the fact event:
  - `rule_id`
  - `severity`
  - `cwe`
  - `owasp_mobile`
  - `masvs`
  - `artifact_target`
  - artifact-scoped `file_path`
  - `line_start`
  - `line_end`
  - `parser_id`
  - `tool_id`
  - `operation`
  - `execution_id`
- Added severity-to-confidence mapping for MobSF findings.
- Parse failure emits one `EventParser` with `status: parse_failed`, preserves the completed tool run, and emits no facts.
- Bounded parser input handling to 1 MiB after transcript capture.
- Treated MobSF-reported file paths as untrusted tool output. Absolute paths, traversal paths, drive-qualified paths, and other unsafe path values collapse to the scoped artifact target instead of being persisted directly in finding statements or `file_path`.

## Not Implemented
- This is not full app testing, full APK/mobile assessment, or a complete static-analysis platform.
- This does not add builders or parsers for `apktool`, `jadx`, `semgrep`, `gitleaks`, `trufflehog`, `binwalk`, `exiftool`, `nmap`, `nuclei`, `naabu`, or `subfinder`.
- This does not bundle real production app/artifact payloads.
- This does not add a broad TUI/browser app-scan launcher.
- The 1 MiB parser cap applies to parser promotion, not to the pre-existing command transcript capture path.

## Validation
- RED first:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestRunWorkspacePromotesMobsfscanJSONToEvidenceFinding' -count=1 -v`
  - Failed because the replay snapshot had no `parser.mobsfscan_json` finding fact.
  - Strengthened structured metadata assertions; failed until `rule_id`, `severity`, `file_path`, `line_start`, and `parser_id` were persisted.
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecutionScopesMobsfscanReportedFilePath' -count=1 -v`
  - Failed because `/etc/passwd` from untrusted MobSF JSON was persisted as `file_path` and leaked into the finding statement.
- Focused GREEN:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Test(ParserEventsForToolExecutionRecordsMobsfscanParseFailureWithoutFact|ParserEventsForToolExecutionScopesMobsfscanReportedFilePath|MobsfscanCleanRelativeArtifactPathRejectsUnsafeSegments|RunWorkspacePromotesMobsfscanJSONToEvidenceFinding|RunWorkspaceRecordsMobsfscanParseFailureWithoutFinding)' -count=1 -v`
- Full repo validation:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
  - `GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
  - `make doctor-strict`
- Hygiene:
  - `gofmt -l internal/app/run.go internal/app/run_test.go internal/app/mobsfscan_parser.go internal/app/mobsfscan_parser_test.go`
  - `git diff --check`
- Stale wording search:
  - `grep -RInE 'artifact tool transcripts are not promoted|transcript-level execution evidence|full app testing|full APK/mobile|complete static-analysis platform|complete app|end-to-end static testing|production app/artifact payloads|ambient PATH fallback' README.md docs/development.md docs/progress/milestone-27-mobsfscan-parser-evidence.md task_plan.md findings.md || true`
  - Matches are limited to explicit Not Implemented / follow-up boundary wording in this milestone file.

## Next Step
- Add parser dispatch for the next high-value managed outputs, likely `nuclei_jsonl`, `naabu_jsonl`, `subfinder_jsonl`, or `nmap_xml`, each with its own output-size, path, redaction, and evidence-boundary tests.
- Continue real managed payload packaging so users do not need to install common binaries manually.
- Add the next artifact/app builder only after its argv, output, and parser/redaction boundary is explicit.

## Risks / Decisions
- Parser promotion is deliberately app-level and narrow for `mobsfscan_json`; the generic compiler parser binding is not reused here because run-side parser failure must not trigger a Sisyphus fallback or fail an otherwise completed tool execution.
- Artifact path confinement remains a filesystem boundary. Parser evidence cannot persist untrusted absolute or traversal paths merely because a tool printed them.
- Parser failure is evidence about a completed run, not a tool execution failure.
