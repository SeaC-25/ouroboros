# Milestone 34: Katana Managed Parser Evidence

## Implemented

- Added a narrow `katana_jsonl` post-tool parser path after completed tool execution events.
- Added a process-mode managed binary argv builder for `katana/crawl` only from explicit `kind: url` scope.
- The builder runs `katana -u <url> -j -silent -or -ob -duc`.
- Successful explicit-URL `katana/crawl` JSONL output emits one `EventParser(status=parsed)` plus ordered `EventFact(kind=surface)` records.
- Replay projection turns those facts into blackboard facts and Evidence Graph surface nodes through the existing event projection path.
- Parse failure emits `EventParser(status=parse_failed, finding_count=0)` and does not fail the completed tool run or create surface facts.
- Missing explicit `kind: url` scope fails closed with `parse_failed` and no facts.
- Domain, host, and CIDR scope do not authorize Katana execution or parser promotion.
- JSONL promotion is capped at 1 MiB of raw tool transcript.
- Accepted route URL fields include top-level `url` / `endpoint` and nested `request.url` / `request.endpoint`, matching Katana's JSON structure.
- Durable surface metadata is limited to parser/tool/execution identity, `url`, `target_url`, `method`, `status_code`, `content_length`, `depth`, `confidence`, and a compact statement.
- Query strings, fragments, `source`, `tag`, `attribute`, headers, raw traffic, request/response/body fields, forms, XHR request details, knowledgebase data, technologies, stored response paths, timestamps, errors, and unknown JSON keys are not persisted in facts.

## Not Implemented

- This is not full Web crawling coverage and does not claim browser/JavaScript crawling completeness.
- This does not bundle a production `katana` binary; execution still requires workspace-managed payload installation.
- This does not add raw request/response storage, form extraction, XHR graphing, technology fingerprint retention, stored response handling, or output-file handling.
- This does not authorize domain, host, or CIDR scope for Katana execution or parser promotion.
- This does not add parser/evidence promotion for `amass`, `dnsrecon`, `ffuf`, `dalfox`, or other remaining tool families.

## Validation

- RED first:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Katana' -count=1 -v`
  - Failed because the dispatcher returned no `katana_jsonl` parser events, replay snapshots had no `parser.katana_jsonl` facts, and the native managed command builder returned no Katana argv.
- Focused GREEN:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Katana' -count=1 -v`
  - Passed after parser, dispatcher, managed argv builder, and run-level integration.
- Parser family regression:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Test(ParserEventsForToolExecution|RunWorkspace|NativeManagedRunCommand).*(Katana|Dnsx|Naabu|Subfinder|Nmap|Httpx|Nuclei|Mobsfscan)' -count=1 -v`
  - Passed across Katana, DNSX, Naabu, Subfinder, Nmap, Httpx, Nuclei, and Mobsfscan parser/run-level regressions.
- Package and full validation:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
  - Passed.
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
  - Passed.
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct make doctor-strict`
  - Passed with `workspace: ready`.
  - `git diff --check`
  - Passed.
  - `gofmt -l internal/app/katana_parser.go internal/app/katana_parser_test.go internal/app/mobsfscan_parser.go internal/app/run.go internal/app/run_test.go`
  - Returned no files.
  - Stale wording search over README, development guide, task plan, findings, progress log, milestone 33, and this milestone file matched only current restricted-scope wording or explicit superseded-history findings.

## Next Step

- Add the next narrow managed argv/parser/evidence promotion only after its scope, parser, file/output, redaction, and packaging boundaries are explicit.

## Risks / Decisions

- Parser promotion is deliberately app-level and narrow for `katana_jsonl`; parser failure is evidence about a completed run, not a tool execution failure.
- URL scope remains the authorization boundary. Domain, host, CIDR, and disabled target policy are not wildcard route-crawl promotion authorization.
- Katana output is tool-controlled and must not become a durable channel for raw traffic, request/response bodies, XHR metadata, stored response paths, timestamps, errors, or unknown JSONL keys.
