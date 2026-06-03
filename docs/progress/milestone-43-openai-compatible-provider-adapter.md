# Milestone 43: OpenAI-Compatible Provider Adapter

## Summary

Milestone 43 adds the first real provider adapter boundary beyond the default offline runtime.

`providers.yaml` now accepts an enabled provider with:

```yaml
type: openai_compatible
model: gpt-4.1
base_url: http://127.0.0.1:8000/v1
api_key_env: OURO_OPENAI_COMPATIBLE_API_KEY
timeout_ms: 5000
```

Before run events are written, the adapter performs:

```text
GET <base_url>/models/<model>
```

It sends `Authorization: Bearer <value>` only when `api_key_env` is configured and the named environment variable is present.

## Execution Boundary

- This is a readiness preflight, not a model generation adapter.
- `base_url` must be an absolute `http` or `https` URL without query or fragment.
- Missing `api_key_env` values fail before any HTTP request is sent.
- Only 2xx responses are accepted as ready.
- Provider errors include provider name, env var name, and status code where relevant, but not bearer token values or response bodies.
- The existing replay model usage event records the configured provider name and model after preflight succeeds.

## Validation

Validated before this milestone record with:

```text
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Provider' -count=1 -v
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'Provider|Model|Replay' -count=1
GOCACHE=/tmp/ouro-go-cache GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict
```

Strict doctor printed `workspace: ready`.

## Remaining Gaps

The IDE is still not complete. Current effective gaps include production payload bundling, provider generation calls and model capability handling beyond readiness preflight, and autonomous distributed worker scheduling beyond bounded/gated audit runs.
