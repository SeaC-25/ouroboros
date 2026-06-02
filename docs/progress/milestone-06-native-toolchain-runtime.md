# Milestone 06: Native Toolchain Runtime

## Implemented
- Added toolchain manifest schema and validation in `internal/toolchain/manifest.go`.
- Added manifest loading from `toolchains/*/manifest.yaml`.
- Added built-in manifest files for `web-api-pack`, `infra-pack`, and `ctf-pack`.
- Added tests in `internal/toolchain/manifest_test.go` for manifest parsing, missing capability rejection, and missing version lock rejection.
- Added Toolchain Health in `internal/toolchain/health.go`.
- Added health states: `ready`, `missing`, `failed`, and `update_available`.
- Added tests in `internal/toolchain/health_test.go` for each health state using test-only runners.
- Added Native Corpus Tools in `internal/toolchain/corpus.go`.
- Added corpus search/read over bundled directories with artifact refs, snippets, bounded results, and path safety checks.
- Added corpus tests in `internal/toolchain/corpus_test.go` plus fixture `test/fixtures/corpus/nuclei/http/test-template.yaml`.

## Not Implemented
- Real tool bundle installation and execution are not implemented yet.

## Validation
- `GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain -v`
- `GOPROXY=https://goproxy.cn,direct go test ./...`

## Next Step
- Continue Milestone 7 with Task 7.1: Compiler Contracts.

## Risks / Decisions
- Manifest versions are explicit version locks at the pack level for now. Per-tool versions can be expanded when concrete native tool bundles are introduced.
- Runner, parser, and healthcheck are declarative strings in this slice; execution behavior belongs to later tool runner integration.
- Health checks use a runner interface. Test doubles remain in tests; production runners can bind to native bundles later.
- Corpus search returns content-addressed artifact refs rather than raw file pointers so later compiler/report layers can treat corpus hits as durable evidence.
