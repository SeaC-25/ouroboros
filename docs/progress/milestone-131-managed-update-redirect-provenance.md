# Milestone 131 - Managed Update Redirect Provenance

## Summary

Milestone 131 keeps managed update downloads on the existing no-follow redirect policy while making rejected redirects auditable.

When `execute-managed-update-download-plan --execute` receives a 3xx response, it now writes a failed download execution audit with:

- `redirect_policy=rejected`
- `redirect_location=<sanitized absolute http(s) URL>`
- `redirect_location_status=recorded|redacted|missing|invalid`

Locations with userinfo, query strings, or fragments are recorded only after those components are removed and are marked `redacted`.

## Implementation

- `ManagedToolBundleUpdateDownloadExecution` now carries redirect provenance fields.
- The download execution HTTP client still returns `http.ErrUseLastResponse` and never follows redirects.
- The 3xx failure branch records sanitized redirect provenance before writing the failed audit.
- CLI `execute-managed-update-download-plan` and `managed-update-download-executions` output include redirect fields.
- Workspace-bound Toolchain Health rows include redirect fields plus `redirect_location_b64` for parser-safe recovery of the sanitized value.
- Runtime Tabs include redirect provenance in read-only `managed-update-download-execution` action output.

## Boundaries

This milestone does not add redirect following, remote trust expansion, payload staging on redirect failure, final bin/rules links, install execution, payload execution, package-manager calls, health probes, provider calls, worker dispatch, endpoint/target contact, request/plan mutation, download-audit mutation after write, or replay/report mutation.

Raw `Location` header values are not written to JSON audits, CLI output, TUI rows, sidecars, or error strings.

## Validation

RED first showed app/CLI/TUI failed because redirect rejection was reported only as a generic HTTP 302 failure and Runtime Tabs ignored redirect fields:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'Redirect|RuntimeManagedUpdateDownloadExecutionUsesEncodedReferenceFields' -count=1 -v
```

GREEN focused validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'Redirect|RuntimeManagedUpdateDownloadExecutionUsesEncodedReferenceFields|ManagedUpdateDownloadExecutionProjectionIncludesEncodedReferenceFields' -count=1 -v
```

Affected package validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -count=1
```

Full validation is recorded in `progress.md`.
