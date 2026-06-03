# Milestone 35: FFUF Managed Parser Evidence

## Implemented

- Added a narrow `ffuf_json` post-tool parser path after completed tool execution events.
- Added a process-mode managed binary argv builder for `ffuf/path_fuzz` only from explicit `kind: url` scope.
- The builder writes `workspace/toolchain/managed-inputs/ffuf/<node>/ffuf-wordlist.txt`.
- The builder runs `ffuf -w <file> -u <url>/FUZZ -json -s`.
- Excluded URL targets and generated wordlist candidate URLs are rejected before the managed wordlist is written.
- Successful explicit-URL `ffuf/path_fuzz` stdout JSON/NDJSON output emits one `EventParser(status=parsed)` plus ordered `EventFact(kind=surface)` records.
- Replay projection turns those facts into blackboard facts and Evidence Graph surface nodes through the existing event projection path.
- Parse failure emits `EventParser(status=parse_failed, finding_count=0)` and does not fail the completed tool run or create surface facts.
- Missing explicit `kind: url` scope fails closed with `parse_failed` and no facts.
- Domain, host, and CIDR scope do not authorize FFUF execution or parser promotion.
- Out-of-scope and excluded result URLs are skipped rather than collapsed.
- JSON/NDJSON promotion is capped at 1 MiB of raw tool transcript.
- Accepted output forms include stdout NDJSON result objects and conservative wrapper JSON with a top-level `results` array.
- Durable surface metadata is limited to parser/tool/execution identity, `url`, `target_url`, `status_code`, `content_length`, `content_words`, `content_lines`, `content_type`, `confidence`, and a compact statement.
- Query strings, fragments, `input`, `position`, `redirectlocation`, `duration`, `resultfile`, `host`, `scraper`, raw request/response/body/header fields, file-output wrapper metadata, and unknown JSON keys are not persisted in facts.

## Not Implemented

- This is not full Web fuzzing coverage and does not claim custom wordlist, recursive fuzzing, auto-calibration, replay proxy, request-file, or output-file support.
- This does not bundle a production `ffuf` binary; execution still requires workspace-managed payload installation.
- This does not add raw request/response storage, scraper result retention, redirect graphing, result-file handling, or custom wordlist artifact handling.
- This does not authorize domain, host, or CIDR scope for FFUF execution or parser promotion.
- This does not add parser/evidence promotion for `amass`, `dnsrecon`, `dalfox`, or other remaining tool families.

## Validation

- RED first:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Ffuf' -count=1 -v`
  - Failed because the dispatcher returned no `ffuf_json` parser events, replay snapshots had no `parser.ffuf_json` facts, and the native managed command builder returned no FFUF argv.
- Focused GREEN:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Ffuf' -count=1 -v`
  - Passed after parser, dispatcher, managed wordlist, managed argv builder, and run-level integration.
- Parser family regression:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Test(ParserEventsForToolExecution|RunWorkspace|NativeManagedRunCommand).*(Ffuf|Katana|Dnsx|Naabu|Subfinder|Nmap|Httpx|Nuclei|Mobsfscan)' -count=1 -v`
  - Passed across FFUF, Katana, DNSX, Naabu, Subfinder, Nmap, Httpx, Nuclei, and Mobsfscan parser/run-level regressions after the review-driven exclusion fix.
- Package and full validation:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
  - Passed.
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
  - Passed.
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct make doctor-strict`
  - Passed with `workspace: ready`.
  - `git diff --check`
  - Passed.
  - `gofmt -l internal/app/ffuf_parser.go internal/app/ffuf_parser_test.go internal/app/mobsfscan_parser.go internal/app/run.go internal/app/run_test.go`
  - Returned no files.
  - Stale wording search over README, development guide, task plan, findings, progress log, milestone 34, and this milestone file matched only current restricted-scope wording, explicit superseded-history findings, or historical progress entries.

## Next Step

- Add the next narrow managed argv/parser/evidence promotion only after its scope, parser, file/output, redaction, and packaging boundaries are explicit.

## Risks / Decisions

- Parser promotion is deliberately app-level and narrow for `ffuf_json`; parser failure is evidence about a completed run, not a tool execution failure.
- URL scope plus URL exclusions remain the authorization boundary. Domain, host, CIDR, and disabled target policy are not wildcard path-fuzz promotion authorization.
- FFUF output is tool-controlled and must not become a durable channel for wordlist input, raw traffic, redirects, scraper output, local result paths, headers, bodies, timestamps, errors, or unknown JSON keys.
