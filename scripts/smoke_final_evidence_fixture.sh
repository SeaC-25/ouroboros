#!/bin/sh
set -eu

# Trusted PATH pinning is opt-in. By default this script keeps the inherited
# PATH so toolchains installed outside the classic system directories (for
# example Go under "C:\Program Files\Go\bin" on Windows Git Bash) remain
# reachable. Set OURO_TRUSTED_PATH to a colon-separated directory list to
# replace PATH with that strict allowlist.
TRUSTED_PATH_DEFAULT="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
TRUSTED_PATH="${OURO_TRUSTED_PATH:-$TRUSTED_PATH_DEFAULT}"
if [ -n "${OURO_TRUSTED_PATH:-}" ]; then
	PATH="$TRUSTED_PATH"
	export PATH
fi

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
TMPDIR_BASE="${TMPDIR:-/tmp}"

fail() {
	printf 'final-evidence-fixture: failed: %s\n' "$1" >&2
	exit 1
}

timestamp_utc() {
	date -u '+%Y-%m-%dT%H:%M:%SZ'
}

find_trusted_shell() {
	for candidate in /bin/sh /usr/bin/sh; do
		if [ -x "$candidate" ]; then
			printf '%s\n' "$candidate"
			return 0
		fi
	done
	return 1
}

file_sha256() {
	path="$1"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$path" | awk '{ print $1 }'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$path" | awk '{ print $1 }'
	else
		fail "sha256sum or shasum is required"
	fi
}

extract_first_yaml_sha256() {
	path="$1"
	awk '
		/^sha256:[[:space:]]*/ {
			value = $0
			sub(/^sha256:[[:space:]]*/, "", value)
			gsub(/"/, "", value)
			gsub(/[[:space:]]/, "", value)
			print value
			found = 1
			exit
		}
		END {
			if (!found) {
				exit 1
			}
		}
	' "$path"
}

assert_sha256() {
	label="$1"
	value="$2"
	printf '%s\n' "$value" | grep -E -q '^[0-9a-fA-F]{64}$' || fail "$label must be a 64-character SHA-256 digest"
	printf '%s\n' "$value" | tr '[:upper:]' '[:lower:]'
}

run_ouro() {
	if [ ! -x "$OURO_BIN" ] && [ -x "$OURO_BIN.exe" ]; then
		OURO_BIN="$OURO_BIN.exe"
	fi
	[ -x "$OURO_BIN" ] || fail "ouro binary missing after fresh build: $OURO_BIN"
	"$OURO_BIN" "$@"
}

case "$TMPDIR_BASE" in
	/*) ;;
	*) fail "TMPDIR must be absolute for final evidence fixture smoke: $TMPDIR_BASE" ;;
esac

mkdir -p "$TMPDIR_BASE"
TRUSTED_SH="$(find_trusted_shell)" || fail "trusted system shell not found"
command -v go >/dev/null 2>&1 || fail "go not found on trusted PATH"

# 优先构建而非 go run:bin 缺失或源码更新时先重建,OURO_SMOKE_SKIP_BUILD=1 可跳过。
OURO_BIN="$ROOT/bin/ouro"
. "$ROOT/scripts/smoke_fresh_bin.sh"
ensure_fresh_ouro_bin "$OURO_BIN" || fail "unable to build fresh ouro binary"

RULE_BUNDLE_DIR="$ROOT/toolchains/managed-bundles/nuclei-templates"
RULE_BUNDLE_MANIFEST="$RULE_BUNDLE_DIR/bundle.yaml"
RULE_PAYLOAD="$RULE_BUNDLE_DIR/payload"
[ -f "$RULE_BUNDLE_MANIFEST" ] || fail "built-in nuclei-templates bundle manifest is missing"
[ -d "$RULE_PAYLOAD" ] || fail "built-in nuclei-templates payload is missing"
RULE_SHA="$(assert_sha256 rule_bundle_sha256 "$(extract_first_yaml_sha256 "$RULE_BUNDLE_MANIFEST")")"

FIXTURE_ROOT="$(mktemp -d "$TMPDIR_BASE/ouro-final-evidence-fixture-XXXXXX")"
EVIDENCE_DIR="$FIXTURE_ROOT/operator-evidence"
PAYLOAD_ROOT="$FIXTURE_ROOT/operator-payloads"
ACCEPTANCE_LOG_DIR="$FIXTURE_ROOT/production-acceptance-logs"
COMPLETION_LOG_DIR="$FIXTURE_ROOT/completion-audit-logs"

mkdir -p "$EVIDENCE_DIR" "$PAYLOAD_ROOT/bin" "$ACCEPTANCE_LOG_DIR" "$COMPLETION_LOG_DIR"

BINARY_PAYLOAD="$PAYLOAD_ROOT/bin/httpx"
{
	printf '#!/usr/bin/env sh\n'
	printf "printf 'production httpx fixture\\\\n'\n"
} > "$BINARY_PAYLOAD"
chmod 755 "$BINARY_PAYLOAD"
BINARY_SHA="$(file_sha256 "$BINARY_PAYLOAD")"

INVENTORY="$EVIDENCE_DIR/payload-inventory.env"
{
	printf 'schema=ouroboros.production_acceptance.inventory.v1\n'
	printf 'inventory_status=operator_owned\n'
	printf 'binary_name=httpx\n'
	printf 'binary_pack=web-api-pack\n'
	printf 'binary_payload=%s\n' "$BINARY_PAYLOAD"
	printf 'binary_expected_sha256=%s\n' "$BINARY_SHA"
	printf 'binary_version=production-httpx-fixture\n'
	printf 'binary_source=final-evidence-fixture/httpx\n'
	printf 'rule_bundle_name=nuclei-templates\n'
	printf 'rule_bundle_pack=web-api-pack\n'
	printf 'rule_bundle_payload=%s\n' "$RULE_PAYLOAD"
	printf 'rule_bundle_expected_sha256=%s\n' "$RULE_SHA"
	printf 'rule_bundle_version=2026.06.04-ouro-seed\n'
	printf 'rule_bundle_source=ouroboros/release-managed-rule-bundle-seed\n'
} > "$INVENTORY"

POLICY="$EVIDENCE_DIR/deployment-policy.env"
{
	printf 'schema=ouroboros.production_acceptance.policy.v1\n'
	printf 'policy_status=approved\n'
	printf 'deployment_environment=fixture\n'
	printf 'accepted_by=final-evidence-fixture\n'
	printf 'accepted_at=%s\n' "$(timestamp_utc)"
	printf 'authorization_scope=fixture-only-local-final-evidence-smoke\n'
	printf 'managed_payload_acceptance_required=true\n'
	printf 'workflow_closure_required=true\n'
	printf 'no_live_targets=true\n'
	printf 'payload_execution_allowed=false\n'
	printf 'network_download_allowed=false\n'
	printf 'package_manager_allowed=false\n'
	printf 'provider_calls_allowed=false\n'
	printf 'worker_contact_allowed=false\n'
} > "$POLICY"

printf 'final-evidence-fixture: root %s\n' "$FIXTURE_ROOT"
printf 'final-evidence-fixture: mode=fixture-only claim_100_ready=false\n'
printf 'final-evidence-fixture: inventory %s\n' "$INVENTORY"
printf 'final-evidence-fixture: policy %s\n' "$POLICY"
printf 'final-evidence-fixture: rule-bundle-sha256 %s\n' "$RULE_SHA"

env \
	PATH="$PATH" \
	GOPROXY="$GOPROXY" \
	TMPDIR="$TMPDIR_BASE" \
	OURO_PRODUCTION_ACCEPTANCE_INVENTORY="$INVENTORY" \
	OURO_PRODUCTION_ACCEPTANCE_POLICY="$POLICY" \
	OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="$ACCEPTANCE_LOG_DIR" \
	"$TRUSTED_SH" "$ROOT/scripts/smoke_production_acceptance.sh"

SUMMARY="$ACCEPTANCE_LOG_DIR/summary.txt"
[ -f "$SUMMARY" ] || fail "production acceptance summary was not written"
SUMMARY_SHA="$(file_sha256 "$SUMMARY")"

ATTESTATION="$EVIDENCE_DIR/completion-attestation.env"
{
	printf 'schema=ouroboros.completion_audit.attestation.v1\n'
	printf 'attestation_status=approved\n'
	printf 'reviewed_by=final-evidence-fixture\n'
	printf 'reviewed_at=%s\n' "$(timestamp_utc)"
	printf 'production_summary_sha256=%s\n' "$SUMMARY_SHA"
	printf 'production_evidence=operator_owned\n'
	printf 'fixture_evidence_allowed=false\n'
	printf 'scope_reviewed=true\n'
	printf 'safety_boundaries_reviewed=true\n'
	printf 'release_managed_reviewed=true\n'
	printf 'managed_payloads_reviewed=true\n'
	printf 'workflow_closure_reviewed=true\n'
	printf 'documentation_reviewed=true\n'
} > "$ATTESTATION"

printf 'final-evidence-fixture: attestation %s\n' "$ATTESTATION"

env \
	PATH="$PATH" \
	GOPROXY="$GOPROXY" \
	TMPDIR="$TMPDIR_BASE" \
	OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="$SUMMARY" \
	OURO_COMPLETION_AUDIT_ATTESTATION="$ATTESTATION" \
	OURO_COMPLETION_AUDIT_LOG_DIR="$COMPLETION_LOG_DIR" \
	"$TRUSTED_SH" "$ROOT/scripts/audit_completion.sh"

run_ouro completion-status "$COMPLETION_LOG_DIR"

ACCEPT_LOG="$FIXTURE_ROOT/completion-accept-fixture.log"
if run_ouro completion-accept "$COMPLETION_LOG_DIR" \
	--reviewed-by final-evidence-fixture \
	--accepted-at "$(timestamp_utc)" \
	--accept-real-evidence > "$ACCEPT_LOG" 2>&1; then
	cat "$ACCEPT_LOG"
	fail "fixture completion evidence must not be final accepted"
fi
if ! grep -q 'fixture evidence cannot be final accepted' "$ACCEPT_LOG"; then
	cat "$ACCEPT_LOG"
	fail "fixture completion acceptance failed for an unexpected reason"
fi
printf 'final-evidence-fixture: completion-accept fixture-rejected\n'

printf 'final-evidence-fixture: completion-log-dir %s\n' "$COMPLETION_LOG_DIR"
printf 'final-evidence-fixture: ok\n'
