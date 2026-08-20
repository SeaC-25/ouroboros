# Milestone 191 - Direct-Use Smoke Path Component Hardening

## Summary

Hardened `scripts/smoke_direct_use.sh` so configured direct-use smoke paths reject existing symlink path components, not only symlink leaves. This covers the built binary path, workspace path, and smoke log directory path before binary execution or smoke evidence writes.

## Behavior

- `OURO_DIRECT_USE_SMOKE_BIN` must be absolute and must not pass through an existing symlink component.
- `WORKSPACE`, when provided, must be absolute and must not pass through an existing symlink component.
- `OURO_DIRECT_USE_SMOKE_LOG_DIR`, when provided, must be absolute and must not pass through an existing symlink component before or after `mkdir -p`.
- Existing direct symlink leaf refusal remains in place.

The default temporary workspace/log paths remain fresh smoke paths. The hardening is focused on preserving reviewed path spelling for operator-configured direct-use evidence inputs.

## Validation

Focused RED/GREEN passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDirectUseSmoke(RejectsSymlinkPathComponents|ScriptExercisesBuiltBinaryPath)' -count=1 -v
```

## Remaining

This is direct-use evidence hardening only. It does not claim final production completion; that still requires operator-owned production evidence, approved attestation, completion audit, manifest/digest sidecar review, and final human/operator acceptance.
