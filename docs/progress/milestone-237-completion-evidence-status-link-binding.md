# Milestone 237: Completion Evidence Status Link Binding

## Summary

Hardened `ouro completion-status <audit-log-dir>` so a digest-consistent evidence set still cannot report `ready-for-final-review` when the files point at the wrong companion artifacts.

The command now verifies that:

- `completion-audit.txt` `evidence_manifest` points to the fixed manifest path being inspected
- `completion-audit.txt` `evidence_manifest_digest` points to the fixed digest-sidecar path being inspected
- `completion-evidence-manifest.txt` `result` points to the fixed result path being inspected
- `completion-evidence-manifest.sha256` `result` points to the fixed result path being inspected
- `completion-evidence-manifest.sha256` `evidence_manifest` points to the fixed manifest path being inspected

When those links are missing or mismatch, the command reports `completion_evidence_link_mismatch` instead of `operator_review_required`.

## Boundary

The command remains read-only. It does not run `make audit-completion`, write evidence, mutate workspaces, call providers, execute tools, dispatch workers, contact targets, sign evidence, or claim 100%.

## Verification

RED first proved a self-consistent but incorrectly linked evidence set still reported `ready-for-final-review`. GREEN binds ready status to the fixed three-file evidence relationship before digest verification and final operator review status.
