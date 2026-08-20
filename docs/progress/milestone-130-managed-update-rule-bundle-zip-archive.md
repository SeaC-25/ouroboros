# Milestone 130 - Managed Update Rule-Bundle Zip Archive Download

## Summary

Milestone 130 extends the managed update rule-bundle archive gate from `tar.gz` to `zip` while preserving the M129 trust split:

```bash
ouro toolchain managed-update-download-plan <workspace> <request-artifact> --plan --source-url <https-url> --expected-sha256 <directory-sha256> --archive-format zip --expected-archive-sha256 <archive-sha256>
ouro toolchain execute-managed-update-download-plan <workspace> <plan-artifact> --execute
ouro toolchain apply-managed-update-download-execution <workspace> <download-execution-artifact> --apply
```

For `rule_bundle`, `expected_sha256` remains the extracted canonical record v1 directory digest. `archive_expected_sha256` is the downloaded zip file digest.

## Implementation

- `archive_format=zip` is accepted only for `rule_bundle` update download plans.
- Download execution writes archive bytes to staging as `archive` and verifies archive SHA-256 before extraction.
- Zip extraction writes only to the same staging directory under a temporary payload directory, then publishes `payload/` only after the extracted directory digest matches.
- Extracted regular files are normalized to `0644`.
- The shared archive path normalizer rejects absolute paths, traversal, backslashes, Windows drive prefixes, UNC-style absolute paths, and any `..` path segment.
- Zip extraction rejects symlinks, non-regular entries, duplicate output paths, and extracted payloads over the managed update byte cap.
- Apply continues to revalidate plan/request/download audit metadata, archive format, archive expected/actual SHA-256, `archive_digest_status=matched`, `archive_digest_scope=file`, and the staged directory digest before writing a workspace managed bundle.

## Boundaries

This milestone does not add redirect following, final `toolchain/rules` links, install execution, payload execution, package-manager calls, health probes, provider calls, worker dispatch, endpoint/target contact, request/plan/download audit mutation, or replay/report mutation.

## Validation

- RED confirmed zip plans failed before implementation with `managed bundle update archive format is unsupported: zip`.
- Focused zip/tar archive validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'ZipRuleBundleArchive|DuplicateZipRuleBundleArchive|RuleBundleArchive|UnsafeRuleBundleArchive|TamperedArchiveMetadata|ExtractsZipRuleBundleArchive' -count=1 -v
```

Full repository validation is recorded in `progress.md`.

- Affected package validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -count=1
```

- Full repository validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-release-managed
sh -n scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_release_managed.sh
git diff --check
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct gofmt -l internal/app/toolchain.go internal/app/toolchain_test.go cmd/ouro/main_test.go
```
