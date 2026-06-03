# Milestone 36: Dalfox Managed Parser Evidence

## Implemented

- Added a narrow `dalfox_json` post-tool parser path after completed tool execution events.
- Added a process-mode managed binary argv builder for `dalfox/xss_scan` only from explicit `kind: url` scope.
- The builder runs `dalfox url <url> --format json --silence --no-color --no-spinner`.
- Excluded URL targets are rejected before argv generation and payload execution.
- Successful explicit-URL `dalfox/xss_scan` stdout JSON/NDJSON output emits one `EventParser(status=parsed)` plus ordered `EventFact(kind=finding)` records.
- Replay projection turns those facts into blackboard facts and Evidence Graph finding nodes through the existing event projection path.
- Parse failure emits `EventParser(status=parse_failed, finding_count=0)` and does not fail the completed tool run or create finding facts.
- Missing explicit `kind: url` scope fails closed with `parse_failed` and no facts.
- Domain, host, and CIDR scope do not authorize Dalfox execution or parser promotion.
- Out-of-scope and excluded reported URLs are skipped rather than collapsed.
- JSON/NDJSON promotion is capped at 1 MiB of raw tool transcript.
- Accepted output forms include stdout NDJSON PoC objects, conservative wrapper JSON with top-level `pocs` or `results` arrays, and Dalfox JSON array output with empty sentinel objects ignored.
- Durable finding metadata is limited to parser/tool/execution identity, `matched_url`, `target_url`, `type`, `inject_type`, `poc_type`, `method`, `param`, `severity`, `cwe`, `confidence`, and a compact statement.
- Query strings, fragments, payloads, evidence snippets, raw requests, raw responses, headers, bodies, message strings, report wrapper data, and unknown JSON keys are not persisted in facts.

## Not Implemented

- This is not full XSS scanning coverage and does not claim DOM/browser crawling, blind-callback testing, custom payload, custom header/cookie, proxy, raw-request, or authenticated workflow support.
- This does not bundle a production `dalfox` binary; execution still requires workspace-managed payload installation.
- This does not add raw request/response storage, payload/evidence retention, message-string retention, output-file handling, HAR/debug handling, or found-action hooks.
- This does not authorize domain, host, or CIDR scope for Dalfox execution or parser promotion.
- This does not add parser/evidence promotion for `amass`, `dnsrecon`, `sqlmap`, `arjun`, `gau`, `waybackurls`, or other remaining tool families.

## Validation

- RED first:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Dalfox' -count=1 -v`
  - Failed because the dispatcher returned no `dalfox_json` parser events, replay snapshots had no `parser.dalfox_json` facts, and the native managed command builder returned no Dalfox argv.
- Focused GREEN:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Dalfox' -count=1 -v`
  - Passed after parser, dispatcher, managed argv builder, and run-level integration.
- Review-driven scope regression:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecutionSkipsOutOfScopeDalfoxFindings|Dalfox' -count=1 -v`
  - RED first proved out-of-scope Dalfox reported URLs were folded into scoped findings; GREEN now skips out-of-scope reported URLs.
- Parser family regression:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Dalfox|Nuclei|Ffuf|Katana|Httpx|Dnsx|Naabu|Subfinder|Nmap|Mobsfscan' -count=1 -v`
  - Passed across Dalfox, Nuclei, FFUF, Katana, Httpx, DNSX, Naabu, Subfinder, Nmap, and Mobsfscan parser/run-level regressions before documentation synchronization.
- Package and full validation:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
  - Passed.
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
  - Passed.
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct make doctor-strict`
  - Passed with `workspace: ready`.
  - `git diff --check`
  - Passed.
  - `gofmt -l internal/app/dalfox_parser.go internal/app/dalfox_parser_test.go internal/app/mobsfscan_parser.go internal/app/run.go internal/app/run_test.go`
  - Returned no files.

## Next Step

- Add the next narrow managed argv/parser/evidence promotion only after its scope, parser, file/output, redaction, and packaging boundaries are explicit.

## Risks / Decisions

- Parser promotion is deliberately app-level and narrow for `dalfox_json`; parser failure is evidence about a completed run, not a tool execution failure.
- URL scope plus URL exclusions remain the authorization boundary. Domain, host, CIDR, and disabled target policy are not wildcard XSS promotion authorization.
- Dalfox output is tool-controlled and must not become a durable channel for payloads, evidence snippets, raw traffic, headers, bodies, callback metadata, local output paths, messages, timestamps, errors, or unknown JSON keys.
