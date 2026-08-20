# Milestone 240: Completion Evidence Status Manual Step Binding

## Summary

Hardened `ouro completion-status <audit-log-dir>` so `ready-for-final-review` requires the fixed manual-review step as well as schema, link, and digest binding.

The command now verifies that:

- `completion-audit.txt`, `completion-evidence-manifest.txt`, and `completion-evidence-manifest.sha256` all carry `remaining_manual_step=claim_100_after_real_operator_evidence_review`

Wrong or missing manual-step fields produce `remaining_manual_step_status=mismatch|missing` and `final_review_status=completion_evidence_manual_step_mismatch`.

## Boundary

The command remains read-only. It does not run `make audit-completion`, write evidence, mutate workspaces, call providers, execute tools, dispatch workers, contact targets, sign evidence, or claim 100%.

## Verification

RED first proved an unsafe manual-step string still reported `ready-for-final-review` when schema, link, and digests matched. GREEN binds ready status to the fixed operator-review manual step before final review.
