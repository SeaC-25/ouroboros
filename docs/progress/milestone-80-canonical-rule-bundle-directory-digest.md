# Milestone 80: Canonical Rule-Bundle Directory Digest

## Scope

Rule-bundle directory SHA-256 now uses canonical record v1 instead of the older sorted path plus NUL plus file bytes concatenation.

The digest input is:

1. A fixed format domain prefix: `ouroboros managed rule_bundle directory digest v1\n`.
2. Files sorted by slash-normalized relative path.
3. For each file: 8-byte big-endian path length, path bytes, 8-byte big-endian file size, and the raw SHA-256 digest of file content.

This keeps the existing empty-directory rejection, symlink rejection, non-regular file rejection, and stable open revalidation from milestone 78.

## Changes

- Updated `digestManagedBundleDirectory` to stream each file into a per-file SHA-256 and write length-delimited canonical records into the directory digest.
- Updated app and CLI test helpers so `--expected-sha256` rule-bundle tests use the new canonical record digest.
- Added regression coverage proving the new digest differs from the legacy NUL concatenation format.
- Added a regression test tying the checked-in `toolchains/managed-bundles/nuclei-templates/bundle.yaml` digest to the production canonical digest.
- Updated the built-in `nuclei-templates` seed digest to `01e5f879e780939e0d1a6bb0204045807a0585e8a5bb608032ae0e8b12c24b57`.
- Updated release/workspace managed status metadata so rule-bundle `digest_scope` is now `directory_canonical_records`.

## Validation

RED first failed with production returning the legacy digest:

```text
expected canonical directory digest 38d1972c5036d5a509e2112e4305ecc30dbf33f975df3578b0798ce0cdc7e9aa, got e9e8a20aa8928b4e86e992264cbb857342dc3cea85c74eabb97180c969c8c868; legacy digest was e9e8a20aa8928b4e86e992264cbb857342dc3cea85c74eabb97180c969c8c868
```

Focused GREEN validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestDigestManagedBundleDirectoryUsesCanonicalRecords|TestBundledNucleiTemplatesManifestUsesCanonicalDirectoryDigest|TestImportManagedToolBundleCopiesLocalRuleBundleDirectory|TestReleaseManagedRuleBundleRequiresDigestAndLinksRulesDirectory' -count=1 -v
```

Additional reviewer-driven CLI coverage passed:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run TestToolchainReleaseManagedStatusCommandReportsRuleBundleCanonicalDigestScope -count=1 -v
```

Final validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build
sh -n scripts/release.sh
git diff --check
gofmt -l internal/app/toolchain.go internal/app/toolchain_test.go cmd/ouro/main_test.go
```

Release smoke also passed with a temporary `DIST_DIR`, and the extracted release reported:

```text
release-managed-status: name=nuclei-templates ... sha256=01e5f879e780939e0d1a6bb0204045807a0585e8a5bb608032ae0e8b12c24b57 digest_status=verified digest_algorithm=sha256 digest_scope=directory_canonical_records
```
