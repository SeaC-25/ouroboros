# Milestone 37: Archive Discovery Managed Parser Evidence

## Status

Complete in feature branch `feat/archive-discovery-managed-parser`.

## Scope

This milestone adds a narrow managed execution and parser evidence path for archived URL discovery:

- `gau/archive_discovery`
- `waybackurls/archive_discovery`
- `parser_id: url_list_text`

The implementation is intentionally scoped to explicit domain targets. URL, host, and CIDR scope do not authorize archive discovery execution or parser promotion.

## Implementation

- Added `url_list_text` parser dispatch for completed archive discovery tool executions.
- Added text URL-list parsing with a 1 MiB transcript cap.
- Promoted only HTTP(S) URLs whose host is the scoped root domain or a child subdomain.
- Stripped query strings and fragments before durable evidence persistence.
- Skipped invalid lines, out-of-scope hosts, excluded hosts/URLs, and duplicate sanitized URLs.
- Added managed argv builders:
  - `gau <domain> --subs --providers wayback,otx,commoncrawl`
  - `waybackurls <domain>`
- Changed catalog and authored registry contracts for archive discovery from URL input to `root_domain` / `domain`.

## Durable Evidence

Promoted `EventFact(kind=surface)` payloads persist only:

- parser/tool/execution identity
- root domain
- sanitized URL
- confidence
- compact archive statement

The parser does not persist query strings, fragments, invalid raw lines, out-of-scope hosts, provider metadata, raw request/response material, or unknown text.

## Validation

Focused validation passed:

- `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'URLList|ArchiveDiscovery' -count=1 -v`
- `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain -run 'ArchiveDiscoveryToolsUseDomainScopeContracts|BuiltInAppSecToolsDeclareToolLevelContracts' -count=1 -v`
- `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain ./internal/compiler -count=1`
- `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'URLList|ArchiveDiscovery|Ffuf|Katana|Dalfox|Subfinder|Dnsx' -count=1`
- `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
- `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
- `GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct make doctor-strict`
- `git diff --check`
- touched-file `gofmt -l`

The first full-suite attempt failed during concurrent verification because `/tmp` was exhausted by the Go build cache (`/usr/bin/ld: final link failed: No space left on device`). After `GOCACHE=/tmp/ouro-go-cache go clean -cache`, `/tmp` had 1.8G available and the full suite passed serially.

## Remaining Gaps

- Real production payload bundling for managed binaries.
- Real provider adapters.
- Container-mode managed execution.
- Autonomous distributed worker scheduling beyond bounded/gated audit runs.
- Remaining parser/argv families such as `amass`, `dnsrecon`, `sqlmap`, and `arjun`.
