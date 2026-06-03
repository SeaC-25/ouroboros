# Milestone 44: Release Install Checksum Verification

## Summary

Milestone 44 hardens the local release install path by making checksum verification part of archive installation.

`scripts/release.sh` already writes a versioned tarball plus `SHA256SUMS`. `scripts/install.sh` now looks for a `SHA256SUMS` file beside a local `.tar.gz` archive and requires a matching checksum entry for that archive before extracting or installing it.

## Execution Boundary

- HTTP(S) install sources remain rejected.
- Directory installs continue to work without checksum lookup.
- Archive installs remain local-only.
- If `SHA256SUMS` is absent beside the archive, the installer preserves the existing local install behavior.
- If `SHA256SUMS` is present, the archive basename must appear in it and the SHA-256 digest must match.
- A missing entry, checksum mismatch, or missing checksum tool fails before archive extraction, binary copy, managed-bundle copy, or workspace writes.

## Validation

Validated before this milestone record with:

```text
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test . -run 'TestInstallScriptRejectsArchiveChecksumMismatch' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test . -run 'Release|InstallScript|MakeRelease' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test . -run 'TestMakeReleaseSupportsRelativeDistDir' -count=1 -v
sh -n scripts/install.sh scripts/release.sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict
git diff --check
gofmt -l release_scripts_test.go
```

Strict doctor printed `workspace: ready`; diff hygiene and touched-file formatting checks returned no output. Two concurrent `TestMakeReleaseSupportsRelativeDistDir` processes also passed after the release test switched to a unique relative temp dist path.

## Remaining Gaps

The IDE is still not complete. Current effective gaps include production payload bundling, provider generation calls and model capability handling beyond readiness preflight, and autonomous distributed worker scheduling beyond bounded/gated audit runs. Release provenance is still local checksum verification, not signature verification, SBOM generation, or multi-platform release signing.
