# Milestone 42: Container Managed Execution

## Summary

Milestone 42 connects catalog tool-level managed argv builders to the existing container isolation runner.

Before this milestone, `execution.isolation: container` forced tool-level managed builders offline. Now registered managed binary operations still build their typed argv, but container isolation skips host workspace binary resolution and passes the inner command to the configured container image.

Example inner command:

```text
httpx -json -u https://app.example
```

The existing container runner wraps that as:

```text
<engine> run --rm ... <image> httpx -json -u https://app.example
```

## Execution Boundary

- Process isolation still resolves the first argv element to a workspace-managed binary before execution.
- Container isolation does not use host ambient `PATH` for the inner tool; the configured image must contain the tool.
- Scope target selection, exclusion checks, command argument validation, attempt directories, timeout handling, container audit fields, and `OURO_*` context injection remain centralized through the command runner.
- Unsupported operations, missing target context, missing dependency contracts, invalid container configuration, and unsafe arguments still fail closed.
- This is not production payload bundling; it is the execution path that lets an operator-provided container image supply managed tool binaries.

## Validation

Validated before this milestone record with:

```text
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNativeManagedRunCommandBuildsToolLevelBuilderForContainerIsolation|TestRunWorkspaceExecutesManagedBinaryBuilderThroughContainerIsolation|TestRunWorkspaceCommandRunnerUsesContainerIsolation' -count=1 -v
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Container|ManagedRunCommand|RunWorkspaceExecutesManaged|ToolLevelBuilder|Sqlmap|Httpx|Nuclei|Mobsfscan' -count=1
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain ./internal/compiler -count=1
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct make doctor-strict
git diff --check
gofmt -l internal/app/run.go internal/app/run_test.go
```

Strict doctor printed `workspace: ready`; diff hygiene and touched-file formatting checks returned no output.

## Remaining Gaps

The IDE is still not complete. Current effective gaps include production payload bundling, real provider adapters, and autonomous distributed worker scheduling beyond bounded/gated audit runs.
