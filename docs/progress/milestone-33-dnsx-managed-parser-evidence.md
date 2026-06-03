# Milestone 33: DNSX Managed Parser Evidence

## Implemented

- Added a narrow `dnsx_jsonl` post-tool parser path after completed tool execution events.
- Added a process-mode managed binary argv builder for `dnsx/dns_probe` only from explicit `kind: domain` scope.
- The builder writes a workspace-confined `toolchain/managed-inputs/dnsx/<node>/dnsx-targets.txt` file and runs `dnsx -j -silent -omit-raw -l <file>`.
- The builder checks domain exclusions before writing the input file, so excluded domains do not create input-file side effects or execute payloads.
- The builder deliberately does not use `dnsx -d`, because official help identifies that flag as a brute-force domain option.
- Successful explicit-domain `dnsx/dns_probe` JSONL output emits one `EventParser(status=parsed)` plus ordered `EventFact(kind=surface)` records.
- Replay projection turns those facts into blackboard facts and Evidence Graph surface nodes through the existing event projection path.
- Parse failure emits `EventParser(status=parse_failed, finding_count=0)` and does not fail the completed tool run or create surface facts.
- Missing explicit `kind: domain` scope fails closed with `parse_failed` and no facts.
- JSONL promotion is capped at 1 MiB of raw tool transcript.
- Accepted record fields are `host` plus A, AAAA, and CNAME arrays.
- A and AAAA values must parse as IPv4 and IPv6 addresses respectively.
- CNAME values must normalize to in-scope, non-excluded DNS names.
- URL, host, and CIDR scope do not authorize dnsx execution or parser promotion.
- Durable surface metadata is limited to parser/tool/execution identity, `domain`, `host`, `record_type`, `value`, optional `address` or `cname`, `confidence`, and a compact statement.
- Resolver lists, raw responses, `all` records, CDN/ASN metadata, timestamps, errors, skipped names, and unknown JSON keys are not persisted in facts.

## Not Implemented

- This is not full DNS enumeration coverage and does not claim live resolver completeness.
- This does not bundle a production `dnsx` binary; execution still requires workspace-managed payload installation.
- This does not add DNS brute force, output-file handling, TXT/MX/NS/SOA/SRV/CAA parsing, CDN/ASN retention, or raw resolver evidence retention.
- This does not authorize URL, host, or CIDR scope for dnsx parser promotion.
- This does not add parser/evidence promotion for `amass`, `dnsrecon`, `ffuf`, `katana`, `dalfox`, or other remaining tool families.

## Validation

- RED first:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution.*Dnsx|TestRunWorkspace.*Dnsx|TestNativeManagedRunCommand.*Dnsx' -count=1 -v`
  - Failed because the dispatcher returned no `dnsx_jsonl` parser events, replay snapshots had no `parser.dnsx_jsonl` facts, and the native managed command builder returned no dnsx argv.
- Focused GREEN:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution.*Dnsx|TestRunWorkspace.*Dnsx|TestNativeManagedRunCommand.*Dnsx' -count=1 -v`
  - Passed after parser, dispatcher, managed input-file argv builder, and run-level integration.
- Parser family regression:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution.*(Dnsx|Naabu|Subfinder|Nmap|Httpx|Nuclei|Mobsfscan)|TestRunWorkspace.*(Dnsx|Naabu|Subfinder|Nmap|Httpx|Nuclei|Mobsfscan)' -count=1 -v`
  - Passed across DNSX, Naabu, Subfinder, Nmap, Httpx, Nuclei, and Mobsfscan parser/run-level regressions.
- Package and full validation:
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
  - Passed.
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
  - Passed.
  - `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct make doctor-strict`
  - Passed with `workspace: ready`.
  - `git diff --check`
  - Passed.
  - `gofmt -l internal/app/dnsx_parser.go internal/app/dnsx_parser_test.go internal/app/mobsfscan_parser.go internal/app/run.go internal/app/run_test.go`
  - Returned no files.
  - Stale wording search over README, development guide, task plan, findings, and this milestone file matched only current restricted-scope wording or explicit superseded-history findings.

## Next Step

- Add the next narrow managed argv/parser/evidence promotion only after its scope, parser, file/output, redaction, and packaging boundaries are explicit.

## Risks / Decisions

- Parser promotion is deliberately app-level and narrow for `dnsx_jsonl`; parser failure is evidence about a completed run, not a tool execution failure.
- Domain scope remains the authorization boundary. URL, host, CIDR, and disabled target policy are not wildcard DNS promotion authorization.
- DNSX output is tool-controlled and must not become a durable channel for resolver internals, raw packets, CDN/ASN enrichment, timestamps, errors, or unknown JSONL keys.
- The managed input file is a workspace-confined execution artifact; excluded domains are checked before it is written.
