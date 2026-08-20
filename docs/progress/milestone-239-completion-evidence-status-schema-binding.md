# Milestone 239: Completion Evidence Status Schema Binding

## Summary

Hardened `ouro completion-status <audit-log-dir>` so `ready-for-final-review` requires the fixed completion evidence schemas, not only ready status, links, and digests.

The command now verifies:

- `completion-audit.txt` has `schema=ouroboros.completion_audit.result.v1`
- `completion-evidence-manifest.txt` has `schema=ouroboros.completion_audit.evidence_manifest.v1`
- `completion-evidence-manifest.sha256` has `schema=ouroboros.completion_audit.evidence_manifest_digest.v1`

Wrong or missing schemas produce `evidence_schema_status=mismatch|missing` and `final_review_status=completion_evidence_schema_mismatch`.

## Boundary

The command remains read-only. It does not run `make audit-completion`, write evidence, mutate workspaces, call providers, execute tools, dispatch workers, contact targets, sign evidence, or claim 100%.

## Verification

RED first proved a digest- and link-consistent evidence set with a wrong result schema still reported `ready-for-final-review`. GREEN binds ready status to the fixed v1 schemas before link and digest checks.
