# Milestone 19: Authored Tool Contracts

## Implemented
- Added strict YAML loading for model-authored native tool specs with known-field validation.
- Added `adapter` contract metadata to `AuthoredToolSpec`:
  - `adapter.id`
  - `adapter.operation`
- Authored tool validation now requires both adapter id and operation.
- Authored tool validation rejects dependency command arrays:
  - `dependencies[].health_command`
  - `dependencies[].install_command`
- CLI validate/write/list/read output now surfaces adapter id and operation so reviewers can see which structured contract the model selected.
- App-layer authored tool artifacts now carry adapter id and operation.
- Existing protections remain in place: `raw_script`, `run_command`, non-native runners, path traversal, symlinked metadata paths, existing tool directories, and manifest drift are rejected.

## Missing
- The adapter registry is not executable yet; `adapter.id` and `adapter.operation` are validated metadata, not runtime dispatch.
- Authored specs do not yet store or verify a spec digest in the generated manifest or execution request.
- Parser registry validation is still pending; parser is still a string contract.
- Model-authored worker tool plans still need to replace command arrays with adapter id, operation, input bindings, parser id, and spec digest.
- Real adapters for `httpx`, `nuclei`, `nmap`, and other managed tools remain pending.

## Validation
- RED checks confirmed authored specs did not require adapter id/operation.
- RED checks confirmed authored dependencies could carry command arrays.
- RED checks confirmed authored YAML accepted unknown fields.
- Focused GREEN checks:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain -run 'AuthoredTool' -count=1`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain ./internal/app ./cmd/ouro -run 'AuthoredTool|authored-tool' -count=1`

## Next Step
- Introduce a typed native adapter registry with allowed operations and input bindings.
- Add parser registry validation and structured parser bindings for high-value managed tool outputs.
- Carry authored spec digests into generated manifests and execution plans so runtime can detect drift.

## Risks And Decisions
- This slice deliberately blocks command arrays inside model-authored specs instead of trying to sanitize them.
- Dependency references remain declarative so authored tools can depend on managed bundles without acquiring installer authority.
- Adapter metadata is now required before execution exists. That makes future runtime work safer because specs already identify a structured execution contract.
