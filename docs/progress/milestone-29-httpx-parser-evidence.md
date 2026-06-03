# Milestone 29: Httpx Parser Evidence

## Implemented

- Added a narrow `httpx_jsonl` post-tool parser path after completed tool execution events.
- Successful URL-scoped `httpx/http_probe` JSONL output emits one `EventParser(status=parsed)` plus ordered `EventFact(kind=surface)` records.
- Replay projection turns those facts into blackboard facts and Evidence Graph surface nodes through the existing event projection path.
- Parse failure emits `EventParser(status=parse_failed)` and does not fail the completed tool run or create surface facts.
- Missing URL scope fails closed with `parse_failed` and no facts.
- HTTP probe URLs are treated as untrusted tool output:
  - in-scope URLs are normalized without userinfo, query strings, or fragments;
  - out-of-scope URLs collapse to the scoped URL target;
  - final/redirect URLs do not authorize new targets.
- Out-of-scope primary probe URLs (`url` / `input`) clear response-derived metadata even when another primary field is in scope.
- `final_url`, `final-url`, and `location` are never durable surface URL sources; records that only contain redirect/final URLs emit no surface fact.
- Durable surface metadata is limited to parser/tool/execution identity, scoped URL, target URL, valid status code, validated content type, and non-negative content length.
- Surface statements avoid titles, webserver labels, technology labels, headers, raw traffic, bodies, request/response blobs, screenshots, redirect targets, credentials, and query tokens.

## Not Implemented

- This is not full Web scanner support and does not claim live target scanning is complete.
- This does not bundle a production `httpx` binary; execution still requires workspace-managed payload installation.
- This does not add `subfinder_jsonl`, `naabu_jsonl`, `nmap_xml`, `ffuf_json`, `katana_jsonl`, or `dalfox_json` evidence promotion.
- This does not expand URL scope into domain, wildcard subdomain, or redirect-target authorization.
- This does not persist raw headers, response bodies, screenshots, titles, webserver labels, technology labels, request/response blobs, or out-of-scope URLs in durable surface facts.

## Validation

- RED first:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution.*Httpx|TestRunWorkspace.*Httpx' -count=1 -v`
  - Failed because `parserEventsForToolExecution` returned no `httpx_jsonl` parser events and replay snapshots had no `parser.httpx_jsonl` facts.
- Focused GREEN:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution.*Httpx|TestRunWorkspace.*Httpx' -count=1 -v`
  - Passed after adding the parser path and run-level promotion.
- Scope/redaction hardening:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution.*Httpx' -count=1 -v`
  - Passed after adding explicit coverage for forbidden fields, out-of-scope redirect response metadata, invalid status codes, and negative content length.
- Final review blocker fix:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution(TreatsOutOfScopeHttpxURLAsUntrustedEvenWithScopedInput|DoesNotPromoteHttpxFinalURLAsSurface)' -count=1 -v`
  - RED first failed because out-of-scope `url` could inherit response metadata through in-scope `input`, and `final_url` alone could produce a durable surface fact.
  - Passed after restricting durable surface URL sources to `url` / `input`, collapsing out-of-scope primary URLs to the scoped target with no response metadata, and keeping `final_url` / `location` as attribution checks only.
- Parser regression:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestParserEventsForToolExecution.*(Httpx|Nuclei|Mobsfscan)|TestRunWorkspace(PromotesHttpxJSONLToEvidenceSurface|RecordsHttpxParseFailureWithoutSurface|PromotesNucleiJSONLToEvidenceFinding|RecordsNucleiParseFailureWithoutFinding|PromotesMobsfscanJSONToEvidenceFinding|RecordsMobsfscanParseFailureWithoutFinding)' -count=1 -v`
  - Passed after final hardening and documentation synchronization.
- App package validation:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
  - Passed after final hardening and documentation synchronization.

- Full repository validation:
  - `GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
  - Passed.
- Strict doctor:
  - `make doctor-strict`
  - Passed with `workspace: ready`.
- Hygiene:
  - `git diff --check`
  - `gofmt -l internal/app/httpx_parser.go internal/app/httpx_parser_test.go internal/app/mobsfscan_parser.go internal/app/nuclei_parser.go internal/app/run_test.go`
  - Both returned clean.
- Stale wording search:
  - Searched current stable docs and this milestone for stale `httpx_jsonl` pending wording and broad full-scanner / all-tools-bundled claims.
  - Matches are limited to explicit negative boundary wording such as "not a claim", "not implemented", and "separate milestones".

Because this milestone document records validation results after the validation commands, one final no-edit verification is being run afterward.

## Next Step

- Add parser/evidence promotion for the next narrow tool output family only after its scope, parser, file/output, redaction, and packaging boundaries are explicit.

## Risks / Decisions

- Parser promotion is deliberately app-level and narrow for `httpx_jsonl`; parser failure is evidence about a completed run, not a tool execution failure.
- HTTP probe output is server/tool-controlled and must not become a new durable secret channel through titles, headers, bodies, redirect targets, or technology labels.
- URL scope remains URL scope. Redirects, final URLs, and URL-only scope do not authorize wildcard domains or subdomains.
