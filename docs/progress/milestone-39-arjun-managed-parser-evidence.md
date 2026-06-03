# Milestone 39: Arjun Managed Parser Evidence

## Summary

Milestone 39 adds narrow explicit-URL managed execution and parser evidence promotion for `arjun/parameter_discovery`.

`arjun` now runs only from `kind: url` scope as:

```text
arjun -u <url> -oJ <workspace>/toolchain/managed-outputs/arjun/<node>.json -q -t 1
```

The tool uses the workspace-confined managed output-file boundary. After command success, the runner promotes the expected managed output file into the parser transcript only when the file exists and is a regular non-symlink file.

## Evidence Boundary

- `arjun_json` promotes only in-scope parameter surface observations from explicit URL scope.
- Out-of-scope and excluded reported URLs are skipped instead of collapsed.
- Query strings, fragments, headers, cookies, raw request/response/body material, passive source output, include data, parameter values, and unknown JSON keys are not persisted.
- Durable facts keep only parser/tool/execution identity, scoped URL, target URL, parameter name, confidence, and compact statement.
- Malformed, oversized, or missing-URL-scope JSON records `parse_failed` without failing the completed tool run or emitting facts.

## Validation

Validated before this milestone record with:

```text
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Arjun' -count=1 -v
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Arjun|Ffuf|Katana|Dalfox|URLList|Httpx|Nuclei' -count=1
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain ./internal/compiler -count=1
```

Full app, full repository, strict doctor, diff hygiene, and final formatting checks remain pending after documentation synchronization.

## Remaining Gaps

The IDE is still not complete. Current effective gaps include production payload bundling, real provider adapters, autonomous distributed worker scheduling beyond bounded/gated audit runs, container-mode managed execution, and the remaining parser/argv family `sqlmap`.
