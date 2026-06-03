# Milestone 30: Nmap Parser Evidence

## Implemented

- Added a narrow `nmap_xml` post-tool parser path after completed tool execution events.
- Successful `nmap/service_fingerprint` XML output emits one `EventParser(status=parsed)` plus ordered `EventFact(kind=surface)` records for allowed open services.
- Replay projection turns those facts into blackboard facts and Evidence Graph surface nodes through the existing event projection path.
- Parse failure emits `EventParser(status=parse_failed, finding_count=0)` and does not fail the completed tool run or create surface facts.
- Missing or unsupported scope kind fails closed with `parse_failed` and no facts.
- Host scope binds surface facts to the scoped host target; XML hostnames and PTR data are not trusted as fact hosts.
- CIDR scope promotes only XML IP addresses inside the scoped CIDR. Out-of-scope and unverifiable XML hosts are skipped rather than collapsed to the scoped CIDR.
- Only `state="open"` ports become surface facts; closed, filtered, open-filtered, and extraports summaries are ignored.
- Durable nmap surface metadata is limited to parser/tool/execution identity, scope kind/target, host, address, address type, protocol, port, open state, service name, service tunnel, and service confidence.
- Surface statements use the compact form `nmap | <host> | <protocol>/<port> | open | service=<service_name>`.
- Raw XML, scanner args/version/timestamps, hostnames/PTR, MAC/vendor, OS match/class, uptime/distance, sequences, traceroute/hops, runstats, extraports counts, port reasons, script/hostscript output, and service product/version/extrainfo/hostname/ostype/devicetype/CPE/fingerprint fields are not persisted.

## Not Implemented

- This is not full Infra scanner coverage and does not claim live target scanning is complete.
- This does not bundle a production `nmap` binary; execution still requires workspace-managed payload installation.
- This does not parse or promote Nmap OS fingerprinting, NSE script output, traceroute topology, hostnames/PTR, MAC/vendor, runstats, or closed/filtered port summaries.
- This does not add `naabu_jsonl`, `subfinder_jsonl`, `ffuf_json`, `katana_jsonl`, `dnsx_jsonl`, or `dnsrecon_json` evidence promotion.
- This does not expand host or CIDR scope into additional authorization. The parser only promotes facts that are already bound to the explicit scope.

## Validation

- Baseline before edits:
  - `GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
  - Passed in the new worktree before parser changes.
- RED first:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution(PromotesNmapOpenServiceForCIDRScope|PromotesNmapOpenServiceForHostScope|SanitizesNmapXMLAndFiltersUnsafeFields|DropsOutOfScopeNmapCIDRHost|IgnoresClosedFilteredAndExtraports|RequiresNmapHostOrCIDRScope|RecordsNmapParseFailureWithoutSurface)|TestRunWorkspace(PromotesNmapXMLToEvidenceSurfaceFromCIDR|PromotesNmapXMLToEvidenceSurfaceFromHost|RecordsNmapParseFailureWithoutSurface)' -count=1 -v`
  - Failed because `parserEventsForToolExecution` returned no `nmap_xml` parser events and replay snapshots had no `parser.nmap_xml` facts.
- Focused GREEN:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution(PromotesNmapOpenServiceForCIDRScope|PromotesNmapOpenServiceForHostScope|SanitizesNmapXMLAndFiltersUnsafeFields|DropsOutOfScopeNmapCIDRHost|IgnoresClosedFilteredAndExtraports|RequiresNmapHostOrCIDRScope|RecordsNmapParseFailureWithoutSurface)|TestRunWorkspace(PromotesNmapXMLToEvidenceSurfaceFromCIDR|PromotesNmapXMLToEvidenceSurfaceFromHost|RecordsNmapParseFailureWithoutSurface)' -count=1 -v`
  - Passed after adding the parser and dispatcher case.
- Parser regression:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution(PromotesNmap|SanitizesNmap|DropsOutOfScopeNmap|IgnoresClosedFilteredAndExtraports|RequiresNmap|RecordsNmap|.*Httpx.*|.*Nuclei.*|.*Mobsfscan.*)|TestRunWorkspace(PromotesNmapXML|RecordsNmapParseFailure|PromotesHttpxJSONL|RecordsHttpxParseFailure|PromotesNucleiJSONL|RecordsNucleiParseFailure|PromotesMobsfscanJSON|RecordsMobsfscanParseFailure)' -count=1 -v`
  - Passed across Nmap, Httpx, Nuclei, and Mobsfscan parser promotion paths.

## Next Step

- Add parser/evidence promotion for the next narrow tool output family only after its scope, parser, file/output, redaction, and packaging boundaries are explicit.

## Risks / Decisions

- Parser promotion is deliberately app-level and narrow for `nmap_xml`; parser failure is evidence about a completed run, not a tool execution failure.
- Nmap XML is scanner-controlled output and must not become a durable side channel for hostnames, PTR records, script output, service versions, OS fingerprints, traceroute paths, MAC vendors, or raw scanner metadata.
- Host/CIDR scope remains explicit authorization. CIDR parser output is skipped when outside scope or unverifiable; it is not collapsed to the CIDR target.
