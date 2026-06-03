# Milestone 31: Subfinder Parser Evidence

## Implemented

- Added a narrow `subfinder_jsonl` post-tool parser path after completed tool execution events.
- Successful explicit-domain `subfinder/subdomain_discovery` JSONL output emits one `EventParser(status=parsed)` plus ordered `EventFact(kind=surface)` records.
- Replay projection turns those facts into blackboard facts and Evidence Graph surface nodes through the existing event projection path.
- Parse failure emits `EventParser(status=parse_failed, finding_count=0)` and does not fail the completed tool run or create surface facts.
- Missing explicit `kind: domain` scope fails closed with `parse_failed` and no facts.
- URL scope does not authorize subfinder promotion, including URL scope with `scope.target_policy: disabled`.
- JSONL promotion is capped at 1 MiB of raw tool transcript.
- Accepted JSONL fields are conservative string fields: `host`, `subdomain`, then `input` or `domain`.
- Promoted DNS names are normalized to lowercase ASCII LDH labels, with one trailing dot trimmed.
- Unsafe values are skipped, including schemes, paths, userinfo, ports, wildcard prefixes, whitespace, control characters, non-ASCII labels, empty labels, overlong labels, and leading or trailing hyphens.
- Out-of-scope, unsafe, and excluded names are skipped rather than collapsed to the scoped domain; all skipped records can still yield `parsed` with count 0.
- Exact domain exclusions and `*.example.com`-style wildcard domain exclusions are enforced at promotion time.
- Durable surface metadata is limited to parser/tool/execution identity, `domain`, `subdomain`, `confidence`, and a compact statement.
- Raw JSONL lines, source metadata, IPs, resolvers, timestamps, errors, tokens, URLs, and unknown JSON keys are not persisted in facts.

## Not Implemented

- This is not full DNS discovery coverage and does not claim live subdomain enumeration is complete.
- This does not bundle a production `subfinder` binary; execution still requires workspace-managed payload installation.
- This does not add IDNA conversion; this slice accepts ASCII LDH labels only.
- This does not add parser/evidence promotion for `amass`, `dnsx`, `dnsrecon`, `ffuf`, `katana`, `dalfox`, or other remaining tool families.
- This does not turn URL scope, disabled target policy, or skipped out-of-scope names into wildcard domain authorization.

## Validation

- RED first:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution.*Subfinder|TestRunWorkspace(PromotesSubfinderJSONLToEvidenceSurface|RecordsSubfinderParseFailureWithoutSurface)' -count=1 -v`
  - Failed because the dispatcher returned no `subfinder_jsonl` parser events and replay snapshots had no `parser.subfinder_jsonl` facts.
- Focused raw-cap RED:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecutionCapsRawSubfinderJSONLOutput' -count=1 -v`
  - Failed because the parser checked the 1 MiB cap after trimming and promoted an oversized raw transcript.
- Focused GREEN:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecutionCapsRawSubfinderJSONLOutput' -count=1 -v`
  - Passed after moving the cap check to raw transcript length.
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Subfinder' -count=1 -v`
  - Passed after parser, dispatcher, and run-level integration.
- Final validation:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution.*(Subfinder|Httpx|Nuclei|Mobsfscan)|TestRunWorkspace(PromotesSubfinderJSONLToEvidenceSurface|RecordsSubfinderParseFailureWithoutSurface|PromotesHttpxJSONLToEvidenceSurface|RecordsHttpxParseFailureWithoutSurface|PromotesNucleiJSONLToEvidenceFinding|RecordsNucleiParseFailureWithoutFinding|PromotesMobsfscanJSONToEvidenceFinding|RecordsMobsfscanParseFailureWithoutFinding)' -count=1 -v`
  - Passed across Subfinder, Httpx, Nuclei, and Mobsfscan parser/run-level regressions.
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
  - Passed.
  - `GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
  - Passed.
  - `make doctor-strict`
  - Passed with `workspace: ready`.
  - `git diff --check`
  - Passed.
  - `gofmt -l internal/app/subfinder_parser.go internal/app/subfinder_parser_test.go internal/app/mobsfscan_parser.go internal/app/run_test.go`
  - Returned no files.

## Next Step

- Add the next narrow parser/evidence promotion only after its scope, parser, file/output, redaction, and packaging boundaries are explicit.

## Risks / Decisions

- Parser promotion is deliberately app-level and narrow for `subfinder_jsonl`; parser failure is evidence about a completed run, not a tool execution failure.
- Domain scope remains domain scope, and URL scope remains URL scope. `scope.target_policy: disabled` is not wildcard subdomain authorization.
- Subfinder output is tool-controlled and must not become a durable channel for source metadata, resolvers, IPs, tokens, URLs, or unknown JSONL keys.
- Milestone 30 is already occupied by the parallel nmap parser evidence worktree, so this branch uses milestone 31.
