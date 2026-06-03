# Milestone 41: Sqlmap Log Artifact Wrapper

## Summary

Milestone 41 makes the `sqlmap/injection_test` managed output boundary work with real sqlmap CLI output, not only with a prewritten Ouroboros JSON artifact.

The runner still prefers:

```text
workspace/toolchain/managed-outputs/sqlmap/<node>/ouro-sqlmap.json
```

When that file is absent, it searches the same managed output directory for exactly one regular non-symlink file named `log`, matching sqlmap's target-specific output layout. That log is converted into the existing `sqlmap_json` parser transcript.

## Evidence Boundary

- The log fallback converts only `Parameter:`, `Type:`, first `Title:`, and back-end DBMS lines.
- Common sqlmap DBMS forms such as `back-end DBMS: PostgreSQL` and `[INFO] the back-end DBMS is PostgreSQL` are supported.
- Symlinked `log` files, non-regular logs, and multiple log files fail closed.
- Payload lines, cookies, authorization headers, raw requests/responses, OS/server details, dump/database/table/column/row material, local output paths, and unknown lines are ignored.
- The existing `sqlmap_json` parser still enforces explicit URL scope, scope/exclusion filtering, durable field allowlisting, query/fragment stripping, and parse-failure-as-parser-event behavior.

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
gofmt -l internal/app/sqlmap_parser.go internal/app/sqlmap_parser_test.go internal/app/run.go internal/app/run_test.go
```

Strict doctor printed `workspace: ready`; diff hygiene and touched-file formatting checks returned no output.

## Remaining Gaps

The IDE is still not complete. Current effective gaps include production payload bundling, real provider adapters, autonomous distributed worker scheduling beyond bounded/gated audit runs, and container-mode managed execution.
