# Milestone 83: TUI Executable Row Control-Field Hardening

Date: 2026-06-05

## Summary

Two TUI metadata-only executable row parsers were hardened after read-only review found that untrusted free text could contain `key=value` fragments:

- `model tool plan preview:` now parses action/path/mode/validity only from the control segment before `preview=`.
- `compile-preview review artifact:` now uses the trailing authoritative `path=` token instead of a `path=` embedded in review reason text.

Both paths remain metadata-only/double-confirm gates as before; this milestone only prevents free-text display fields from retargeting the artifact path or changing executable state.

## Validation

RED tests first reproduced the retargeting issue:

- A preview string containing `path=... mode=process validity=invalid` retargeted the model tool-plan preview action.
- A review reason containing `path=...` retargeted the compile-preview review artifact action.

After the fix, focused validation passed:

```sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app -run 'TestRuntime(ModelToolPlanPreviewIgnoresControlKeysInPreviewText|CompilePreviewReviewArtifactIgnoresPathInReasonText)|ModelToolPlanPreview|CompilePreviewReviewArtifact|NewTUIModelForWorkspaceShowsReleaseManagedStatusRowsFromSourceReadOnly' -count=1 -v
```

Full repository validation is pending after this document write.

## Remaining Work

- A broader row-value encoder or structured action metadata model would make all CLI/TUI `key=value` rows consistent, especially read-only rows with whitespace-bearing display values.
