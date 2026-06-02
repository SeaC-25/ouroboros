# Milestone 12: Build And Distribution

## Implemented
- Added `Makefile` with `test`, `build`, `doctor`, `release`, and `clean` targets.
- Added `scripts/build.sh` to build `bin/ouro` from `./cmd/ouro`.
- Added `scripts/release.sh` to build a versioned `dist/ouro_<version>_<goos>_<goarch>.tar.gz` archive plus `SHA256SUMS`.
- Added `scripts/install.sh` to install from a local release archive or extracted release directory into `${PREFIX:-$HOME/.local}/bin/ouro`.
- Added `.github/workflows/ci.yml` to run `make test`, `make build`, and `make release`.
- `make test` runs `GOPROXY=https://goproxy.cn,direct go test ./...`.
- `make build` builds an executable `bin/ouro`.
- `make release` builds a local release archive and checksum file using overrideable `VERSION`, `DIST_DIR`, `GOOS`, `GOARCH`, and `GOPROXY`.
- `make doctor` creates a temporary workspace, initializes it, and runs `ouro doctor`.
- Added `docs/development.md` with repository layout, build/test commands, extension points, and safety notes.
- Updated `AGENTS.md` to reflect the current implementation structure, Makefile commands, run command, and extension locations.

## Not Implemented
- Multi-platform release matrix uploads and signed release provenance are not implemented yet.

## Validation
- `make test`
- `make build`
- `make release`
- `test -f dist/SHA256SUMS`
- `test -x bin/ouro`
- `./bin/ouro --help`
- `make doctor`
- `GOPROXY=https://goproxy.cn,direct go test . -run 'ReleaseScript|InstallScript|ReleaseMakefile' -v`
- `GOPROXY=https://goproxy.cn,direct go test ./...`

## Next Step
- Continue with a completion audit against the cross-cutting acceptance criteria and identify the next production-hardening milestone.

## Risks / Decisions
- `make doctor` uses a temporary workspace so it can run in a clean checkout without creating persistent local state.
- `scripts/build.sh` respects `GOPROXY`, `BIN_DIR`, and `BIN` so local and CI builds can override paths without editing the script.
- `scripts/release.sh` is local and deterministic: it builds one target tuple, writes an archive and checksum file, and does not publish artifacts.
- `scripts/install.sh` deliberately rejects HTTP(S) sources and installs only from an operator-provided local archive or directory.
- Documentation now treats `model/` as immutable reference input and `internal/` as the implementation surface.
