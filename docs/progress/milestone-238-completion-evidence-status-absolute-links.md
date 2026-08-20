# Milestone 238: Completion Evidence Status Absolute Links

## Summary

Hardened `ouro completion-status <audit-log-dir>` so evidence link fields must be absolute paths, matching the shape written by `make audit-completion`.

The command now rejects `ready-for-final-review` when any fixed link field is relative, even if that relative path resolves to the same file from the current process directory.

Checked fields remain:

- `completion-audit.txt` `evidence_manifest`
- `completion-audit.txt` `evidence_manifest_digest`
- `completion-evidence-manifest.txt` `result`
- `completion-evidence-manifest.sha256` `result`
- `completion-evidence-manifest.sha256` `evidence_manifest`

Relative links produce `evidence_link_status=relative` and `final_review_status=completion_evidence_link_mismatch`.

## Boundary

The command remains read-only. It does not run `make audit-completion`, write evidence, mutate workspaces, call providers, execute tools, dispatch workers, contact targets, sign evidence, or claim 100%.

## Verification

RED first proved a relative link that resolved to the inspected manifest still reported `ready-for-final-review`. GREEN rejects non-absolute link fields before path equivalence checks.
