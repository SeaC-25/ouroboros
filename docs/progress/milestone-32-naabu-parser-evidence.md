# Milestone 32: Naabu Parser Evidence

## Implemented

- Added a narrow `naabu_jsonl` post-tool parser path after completed tool execution events.
- Successful CIDR-scoped `naabu/port_scan` JSONL output emits one `EventParser(status=parsed)` plus ordered `EventFact(kind=surface)` records.
- Replay projection turns those facts into blackboard facts and Evidence Graph surface nodes through the existing event projection path.
- Parse failure emits `EventParser(status=parse_failed, finding_count=0)` and does not fail the completed tool run or create surface facts.
- Missing explicit `kind: cidr` scope fails closed with `parse_failed` and no facts.
- JSONL promotion is capped at 1 MiB of raw tool transcript.
- Accepted IP fields are `ip` or IP-valued `host`; DNS hostnames are ignored.
- Accepted port fields are numeric or string `port` values from 1 through 65535.
- Protocol defaults to `tcp`; explicit `udp` is accepted.
- IPv6 endpoints are formatted with brackets, for example `[2001:db8::12]:8443`.
- IPs outside the scoped CIDR and IPs matched by scope exclusions are skipped rather than collapsed.
- Durable surface metadata is limited to parser/tool/execution identity, `target_cidr`, `ip`, `port`, `protocol`, `endpoint`, `confidence`, and a compact statement.
- Hostnames, timestamps, CDN flags, banners, errors, raw/request/response fields, and unknown JSON keys are not persisted in facts.

## Not Implemented

- This is not full port scanning coverage and does not claim live network enumeration completeness.
- This does not bundle a production `naabu` binary; execution still requires workspace-managed payload installation.
- This does not add service detection, banner retention, UDP coverage claims, or output-file handling.
- This does not authorize host scope for naabu parser promotion; the current parser boundary is explicit CIDR scope only.
- This does not add parser/evidence promotion for `amass`, `dnsx`, `dnsrecon`, `ffuf`, `katana`, `dalfox`, or other remaining tool families.

## Validation

- RED first:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution.*Naabu|TestRunWorkspace.*Naabu' -count=1 -v`
  - Failed because the dispatcher returned no `naabu_jsonl` parser events and replay snapshots had no `parser.naabu_jsonl` facts.
- Focused GREEN:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution.*Naabu|TestRunWorkspace.*Naabu' -count=1 -v`
  - Passed after parser, dispatcher, and run-level integration.
- Parser family regression:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution.*(Naabu|Subfinder|Nmap|Httpx|Nuclei|Mobsfscan)|TestRunWorkspace(PromotesNaabuJSONLToEvidenceSurface|RecordsNaabuParseFailureWithoutSurface|PromotesSubfinderJSONLToEvidenceSurface|RecordsSubfinderParseFailureWithoutSurface|PromotesNmapXMLToEvidenceSurfaceFromCIDR|PromotesNmapXMLToEvidenceSurfaceFromHost|RecordsNmapParseFailureWithoutSurface|PromotesHttpxJSONLToEvidenceSurface|RecordsHttpxParseFailureWithoutSurface|PromotesNucleiJSONLToEvidenceFinding|RecordsNucleiParseFailureWithoutFinding|PromotesMobsfscanJSONToEvidenceFinding|RecordsMobsfscanParseFailureWithoutFinding)' -count=1 -v`
  - Passed across Naabu, Subfinder, Nmap, Httpx, Nuclei, and Mobsfscan parser/run-level regressions.
- Package and full validation:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
  - Passed.
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
  - Passed.
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct make doctor-strict`
  - Passed with `workspace: ready`.
  - `git diff --check`
  - Passed.
  - `gofmt -l internal/app/naabu_parser.go internal/app/naabu_parser_test.go internal/app/mobsfscan_parser.go internal/app/run_test.go`
  - Returned no files.

## Next Step

- Add the next narrow parser/evidence promotion only after its scope, parser, file/output, redaction, and packaging boundaries are explicit.

## Risks / Decisions

- Parser promotion is deliberately app-level and narrow for `naabu_jsonl`; parser failure is evidence about a completed run, not a tool execution failure.
- CIDR scope remains the authorization boundary. Host scope, URL scope, and disabled target policy are not port-scan promotion authorization.
- Naabu output is tool-controlled and must not become a durable channel for DNS labels, banners, timestamps, CDN metadata, errors, raw traffic, or unknown JSONL keys.
