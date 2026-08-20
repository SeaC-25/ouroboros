# Milestone 248: Final Evidence Fixture Chat Prompt Chain

## Summary

Re-ran the final evidence fixture after binding the direct chat prompt marker into the AI-provider smoke and completion audit chain.

The fixture proved that the current evidence mechanics still close end to end:

- production acceptance reached `status=accepted mode=operator-owned`;
- completion audit reached `status=ready-for-final-review`;
- `completion-status` reported `final_review_status=fixture_evidence_not_acceptable`;
- `completion-status` kept `claim_100_ready=false`;
- `completion-accept` rejected fixture evidence;
- the canonical IDE AI-provider child log contained `chat-prompt: preview=created stored=false`;
- the production summary and completion evidence manifest both bound the IDE AI-provider child log path and SHA-256.

## Evidence

`make smoke-final-evidence-fixture` passed and printed:

```text
ide-smoke: ok
release-managed: ok
managed-payloads: ok
workflow-closure: ok
production-acceptance: status=accepted mode=operator-owned
completion-audit: status=ready-for-final-review
completion-status: status=ready-for-final-review ... final_review_status=fixture_evidence_not_acceptable claim_100_ready=false ... fixture_evidence_detected=true
final-evidence-fixture: completion-accept fixture-rejected
final-evidence-fixture: ok
```

Direct child-log inspection confirmed:

```text
chat-prompt: preview=created stored=false
ai-provider-smoke: ok
```

The production summary and completion evidence manifest recorded matching `ide_ai_provider_log` and `ide_ai_provider_log_sha256` fields for that child log.

## Boundary

This milestone is fixture evidence only. It proves the final evidence machinery and the direct-chat prompt marker binding, but it is not real operator-owned production evidence and it does not claim 100%.

True completion still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final operator acceptance.

## Validation

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-final-evidence-fixture
```

passed.

Additional inspection:

```bash
grep -n 'chat-prompt\|ai-provider-smoke\|chat-preview\|chat: status' /tmp/ouro-final-evidence-fixture-htQKEl/production-acceptance-logs/ide-smoke-logs/ai-provider.txt
grep -n 'ide_ai_provider_log\|ide_ai_provider_log_sha256\|ide_ai_provider_script' /tmp/ouro-final-evidence-fixture-htQKEl/production-acceptance-logs/summary.txt /tmp/ouro-final-evidence-fixture-htQKEl/completion-audit-logs/completion-evidence-manifest.txt
```

confirmed the direct-chat prompt marker and the final-evidence path/digest bindings.
