# Milestone 28: Nuclei Parser Evidence

## Implemented

- Added a narrow `nuclei_jsonl` post-tool parser path after completed tool execution events.
- Successful URL-scoped `nuclei/template_scan` JSONL output emits one `EventParser(status=parsed)` plus ordered `EventFact(kind=finding)` records.
- Replay projection turns those facts into blackboard facts and Evidence Graph finding nodes through the existing event projection path.
- Parse failure emits `EventParser(status=parse_failed)` and does not fail the completed tool run or create finding facts.
- `matched-at` is treated as untrusted tool output: out-of-scope values collapse to the scoped URL target.
- Finding statements avoid `curl-command`, extracted result values, host/template URL noise, and request/response blobs.
- Structured fact metadata includes parser/tool/operation/execution identity plus template id, severity, matcher/type, scoped matched URL, target URL, CWE, and CVE.

## Not Implemented

- This is not full Nuclei scanner support and does not claim live target scanning is complete.
- This does not bundle production `nuclei` binaries or production `nuclei-templates`; execution still requires workspace-managed payload installation.
- This does not add `httpx_jsonl`, `subfinder_jsonl`, `naabu_jsonl`, or `nmap_xml` evidence promotion.
- This does not expand URL scope into domain or wildcard subdomain authorization.
- This does not persist raw request/response blobs, curl commands, extracted secrets, or out-of-scope matched URLs.

## Validation

- Passed: `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution.*(Nuclei|Mobsfscan)|TestRunWorkspace(PromotesNucleiJSONLToEvidenceFinding|RecordsNucleiParseFailureWithoutFinding|PromotesMobsfscanJSONToEvidenceFinding|RecordsMobsfscanParseFailureWithoutFinding)' -count=1 -v`

Full package validation, strict doctor, whitespace checks, and stale wording checks are pending after documentation synchronization.

## Next Step

- Run full validation and stale wording checks.
- Add parser/evidence promotion for the next narrow tool output family only after its scope, parser, file/output, and redaction boundaries are explicit.
