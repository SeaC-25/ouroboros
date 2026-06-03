# Milestone 40: Sqlmap Managed Parser Evidence

## Summary

Milestone 40 adds narrow explicit-URL managed execution and parser evidence promotion for `sqlmap/injection_test`.

`sqlmap` now runs only from `kind: url` scope as:

```text
sqlmap -u <url> --batch --level=1 --risk=1 --technique=BEU --disable-coloring --output-dir <workspace>/toolchain/managed-outputs/sqlmap/<node> --flush-session
```

The tool uses a workspace-confined managed output-directory boundary. After command success, the runner promotes only the fixed `ouro-sqlmap.json` artifact from that directory when it exists and is a regular non-symlink file.

## Evidence Boundary

- `sqlmap_json` promotes only in-scope SQL injection findings from explicit URL scope.
- Out-of-scope and excluded reported URLs are skipped instead of collapsed.
- Weak non-vulnerable records without injection signal are ignored.
- Query strings, fragments, payloads, headers, cookies, raw request/response/body material, traffic logs, dump/database/table/column/row data, session/cache internals, local output paths, and unknown JSON keys are not persisted.
- Durable facts keep only parser/tool/execution identity, matched URL, target URL, parameter, place, technique, DBMS, title, confidence, and compact statement.
- Malformed, oversized, or missing-URL-scope JSON records `parse_failed` without failing the completed tool run or emitting facts.

## Validation

Validated before this milestone record with:

```text
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Sqlmap' -count=1 -v
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Sqlmap|Dalfox|Arjun|Ffuf|Katana|URLList|Amass|Dnsrecon|Httpx|Nuclei' -count=1
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain ./internal/compiler -count=1
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct make doctor-strict
git diff --check
gofmt -l internal/app/sqlmap_parser.go internal/app/sqlmap_parser_test.go internal/app/run.go internal/app/run_test.go internal/app/mobsfscan_parser.go
```

Strict doctor printed `workspace: ready`; diff hygiene and touched-file formatting checks returned no output.

## Remaining Gaps

The IDE is still not complete. Current effective gaps include production payload bundling, production-grade sqlmap artifact wrapping for real CLI output, real provider adapters, autonomous distributed worker scheduling beyond bounded/gated audit runs, and container-mode managed execution.
