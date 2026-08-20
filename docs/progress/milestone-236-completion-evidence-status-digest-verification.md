# Milestone 236: Completion Evidence Status Digest Verification

## Summary

Hardened `ouro completion-status <audit-log-dir>` so `ready-for-final-review` requires more than three files declaring ready status.

The command now recomputes SHA-256 for:

- `completion-audit.txt`
- `completion-evidence-manifest.txt`

It compares the actual result digest with both the evidence manifest and digest sidecar, compares the actual evidence-manifest digest with the digest sidecar, and reports `completion_evidence_digest_mismatch` instead of `operator_review_required` when recorded values drift.

## Boundary

The command remains read-only. It does not run `make audit-completion`, write evidence, mutate workspaces, call providers, execute tools, dispatch workers, contact targets, sign evidence, or claim 100%.

## Verification

RED first proved a mismatched digest sidecar still reported `ready-for-final-review`. GREEN binds ready status to matched result and manifest digests while still reporting recorded and actual digest values for operator review.
