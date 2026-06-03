# Milestone 38: Amass And DNSRecon Managed Parser Evidence

## Summary

Milestone 38 adds narrow explicit-domain managed execution and parser evidence promotion for `amass/subdomain_discovery` and `dnsrecon/dns_probe`.

`amass` now runs only from `kind: domain` scope as:

```text
amass enum -passive -nocolor -d <domain> -json <workspace>/toolchain/managed-outputs/amass/<node>.json
```

`dnsrecon` now runs only from `kind: domain` scope as:

```text
dnsrecon -d <domain> -t std -j <workspace>/toolchain/managed-outputs/dnsrecon/<node>.json
```

Both tools use a workspace-confined managed output-file boundary. After command success, the runner promotes the expected managed output file into the parser transcript only when the file exists and is a regular non-symlink file.

## Evidence Boundary

- `amass_json` accepts JSON arrays or JSONL objects and promotes only in-scope subdomain names under the explicit root domain.
- `dnsrecon_json` accepts JSON arrays or wrapper objects and promotes in-scope `A`, `AAAA`, `CNAME`, and `NS` DNS observations.
- Both parsers reject missing domain scope, malformed input, and oversized transcripts through `parse_failed` parser events without failing the completed tool run.
- Both parsers skip out-of-scope and excluded names instead of collapsing them into authorized targets.
- Durable facts keep only parser/tool/execution identity and compact surface metadata. Raw tool output, scanner metadata, source strings, timestamps, secrets, zone-transfer material, raw traffic, and unknown fields are not persisted.

## Validation

Validated before this milestone record with:

```text
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Amass|Dnsrecon|DomainFileOutput|DomainToolsOffline' -count=1 -v
```

Full app, full repository, strict doctor, diff hygiene, and final formatting checks remain pending after documentation synchronization.

## Remaining Gaps

The IDE is still not complete. Current effective gaps include production payload bundling, real provider adapters, autonomous distributed worker scheduling beyond bounded/gated audit runs, container-mode managed execution, and remaining parser/argv families such as `sqlmap` and `arjun`.
