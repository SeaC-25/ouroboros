# Milestone 235: Completion Evidence Status CLI

## Summary

Added `ouro completion-status <audit-log-dir>` as a read-only final-review status surface for the completion-audit evidence directory.

The command checks only:

- `completion-audit.txt`
- `completion-evidence-manifest.txt`
- `completion-evidence-manifest.sha256`

It reports missing, partial, or `ready-for-final-review` posture, prints available result and evidence-manifest SHA-256 values, rejects symlinked log/evidence paths, and always keeps `claim_100_ready=false`.

## Boundary

This command does not run `make audit-completion`, write evidence, mutate workspaces, call providers, execute tools, dispatch workers, contact targets, sign evidence, or claim 100%.

The remaining completion gate is still external operator work: production acceptance evidence, approved attestation, completion audit, manifest/digest sidecar review, and final human/operator acceptance.

## Verification

Focused and adjacent validation are run after documentation synchronization. The initial focused RED/GREEN cycle proved the command was absent before implementation and that ready/missing evidence states are reported without creating files.
