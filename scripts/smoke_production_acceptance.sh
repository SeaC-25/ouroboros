#!/bin/sh
set -eu

TRUSTED_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH="$TRUSTED_PATH"
export PATH

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
TMPDIR_BASE="${TMPDIR:-/tmp}"

PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR_CONFIGURED=1
if [ -z "${OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR:-}" ]; then
	PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR_CONFIGURED=0
	OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="$(mktemp -d "$TMPDIR_BASE/ouro-production-acceptance-smoke-logs-XXXXXX")"
fi

SUMMARY_PATH="$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/summary.txt"
SUMMARY_OUTPUT_VERIFIED=0
STALE_STAGE_LOG_CLEANUP_VERIFIED=0
SUMMARY_TMP_PATH=""
RELEASE_MANAGED_LOG="$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/release-managed.txt"
MANAGED_PAYLOADS_LOG="$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/managed-payloads.txt"
WORKFLOW_CLOSURE_LOG="$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/workflow-closure.txt"
IDE_SMOKE_LOG="$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/ide-smoke.txt"
IDE_SMOKE_LOG_TMP_PATH=""
RELEASE_MANAGED_LOG_TMP_PATH=""
MANAGED_PAYLOADS_LOG_TMP_PATH=""
WORKFLOW_CLOSURE_LOG_TMP_PATH=""
IDE_SMOKE_LOG_PUBLISHED=0
RELEASE_MANAGED_LOG_PUBLISHED=0
MANAGED_PAYLOADS_LOG_PUBLISHED=0
WORKFLOW_CLOSURE_LOG_PUBLISHED=0

cleanup_production_acceptance_summary_temp() {
	if [ -n "$SUMMARY_TMP_PATH" ]; then
		rm -f "$SUMMARY_TMP_PATH" 2>/dev/null || true
	fi
	if [ -n "$RELEASE_MANAGED_LOG_TMP_PATH" ]; then
		rm -f "$RELEASE_MANAGED_LOG_TMP_PATH" 2>/dev/null || true
	fi
	if [ -n "$MANAGED_PAYLOADS_LOG_TMP_PATH" ]; then
		rm -f "$MANAGED_PAYLOADS_LOG_TMP_PATH" 2>/dev/null || true
	fi
	if [ -n "$WORKFLOW_CLOSURE_LOG_TMP_PATH" ]; then
		rm -f "$WORKFLOW_CLOSURE_LOG_TMP_PATH" 2>/dev/null || true
	fi
	if [ -n "$IDE_SMOKE_LOG_TMP_PATH" ]; then
		rm -f "$IDE_SMOKE_LOG_TMP_PATH" 2>/dev/null || true
	fi
}

trap cleanup_production_acceptance_summary_temp EXIT HUP INT TERM

remove_stale_stage_log() {
	path="$1"
	[ ! -L "$path" ] || return 0
	if [ -e "$path" ] && [ -f "$path" ]; then
		rm -f "$path" 2>/dev/null || true
	fi
}

remove_stale_unpublished_stage_logs() {
	[ "$STALE_STAGE_LOG_CLEANUP_VERIFIED" = "1" ] || return 0
	[ "$IDE_SMOKE_LOG_PUBLISHED" = "1" ] || remove_stale_stage_log "$IDE_SMOKE_LOG"
	[ "$RELEASE_MANAGED_LOG_PUBLISHED" = "1" ] || remove_stale_stage_log "$RELEASE_MANAGED_LOG"
	[ "$MANAGED_PAYLOADS_LOG_PUBLISHED" = "1" ] || remove_stale_stage_log "$MANAGED_PAYLOADS_LOG"
	[ "$WORKFLOW_CLOSURE_LOG_PUBLISHED" = "1" ] || remove_stale_stage_log "$WORKFLOW_CLOSURE_LOG"
}

timestamp_utc() {
	date -u '+%Y-%m-%dT%H:%M:%SZ'
}

STARTED_AT="$(timestamp_utc)"
AUDIT_ID="${OURO_PRODUCTION_ACCEPTANCE_AUDIT_ID:-production-acceptance-$(date -u '+%Y%m%dT%H%M%SZ')-$$}"

write_failed_summary() {
	[ "$SUMMARY_OUTPUT_VERIFIED" = "1" ] || return 0
	[ ! -L "$SUMMARY_PATH" ] || return 0
	if [ -e "$SUMMARY_PATH" ] && [ ! -f "$SUMMARY_PATH" ]; then
		return 0
	fi
	reason="$(printf '%s' "$1" | tr '\n' ' ')"
	SUMMARY_TMP_PATH="$(mktemp "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/.summary.txt.XXXXXX" 2>/dev/null)" || return 0
	if {
		printf 'schema=ouroboros.production_acceptance.audit.v1\n'
		printf 'audit_id=%s\n' "$AUDIT_ID"
		printf 'started_at=%s\n' "$STARTED_AT"
		printf 'completed_at=%s\n' "$(timestamp_utc)"
		printf 'status=failed\n'
		printf 'mode=operator-owned\n'
		printf 'failure_reason=%s\n' "$reason"
	} > "$SUMMARY_TMP_PATH" 2>/dev/null; then
		mv -f "$SUMMARY_TMP_PATH" "$SUMMARY_PATH" 2>/dev/null || rm -f "$SUMMARY_TMP_PATH" 2>/dev/null || true
	else
		rm -f "$SUMMARY_TMP_PATH" 2>/dev/null || true
	fi
	SUMMARY_TMP_PATH=""
}

fail() {
	write_failed_summary "$1"
	remove_stale_unpublished_stage_logs
	printf 'production-acceptance-smoke: failed: %s\n' "$1" >&2
	printf 'production-acceptance-smoke: logs %s\n' "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR" >&2
	exit 1
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

TRUSTED_SH="$(find_trusted_shell)" || fail "trusted system shell not found"
command -v go >/dev/null 2>&1 || fail "go not found on trusted PATH"

assert_not_symlink_file() {
	label="$1"
	path="$2"
	[ ! -L "$path" ] || fail "$label file must not be a symlink: $path"
}

assert_not_symlink_dir() {
	label="$1"
	path="$2"
	[ ! -L "$path" ] || fail "$label directory must not be a symlink: $path"
}

assert_no_symlink_path_components() {
	label="$1"
	path="$2"
	remaining="${path#/}"
	current=""
	while [ -n "$remaining" ]; do
		component="${remaining%%/*}"
		if [ "$remaining" = "$component" ]; then
			remaining=""
		else
			remaining="${remaining#*/}"
		fi
		[ -n "$component" ] || continue
		current="$current/$component"
		[ ! -L "$current" ] || fail "$label path component must not be a symlink: $current"
		[ -e "$current" ] || break
	done
}

assert_absolute_path() {
	label="$1"
	path="$2"
	case "$path" in
		/*) ;;
		*) fail "$label path must be absolute: $path" ;;
	esac
}

assert_output_file_path() {
	label="$1"
	path="$2"
	assert_not_symlink_file "$label" "$path"
	if [ -e "$path" ] && [ ! -f "$path" ]; then
		fail "$label file must be a regular file or absent: $path"
	fi
}

assert_absolute_path "production acceptance log dir" "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR"
assert_not_symlink_dir "production acceptance log dir" "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR"
if [ "$PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR_CONFIGURED" = "1" ]; then
	assert_no_symlink_path_components "production acceptance log dir" "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR"
fi
mkdir -p "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR"
assert_not_symlink_dir "production acceptance log dir" "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR"
if [ "$PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR_CONFIGURED" = "1" ]; then
	assert_no_symlink_path_components "production acceptance log dir" "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR"
fi
STALE_STAGE_LOG_CLEANUP_VERIFIED=1
assert_output_file_path "production acceptance summary" "$SUMMARY_PATH"
SUMMARY_OUTPUT_VERIFIED=1

workspace_provided=0
if [ -z "${WORKSPACE:-}" ]; then
	SMOKE_ROOT="$(mktemp -d "$TMPDIR_BASE/ouro-production-acceptance-smoke-XXXXXX")"
	SMOKE_ROOT="$(CDPATH= cd -- "$SMOKE_ROOT" && pwd -P)"
	WORKSPACE="$SMOKE_ROOT/workspace"
else
	workspace_provided=1
fi

assert_absolute_path "production acceptance workspace" "$WORKSPACE"
assert_not_symlink_dir "production acceptance workspace" "$WORKSPACE"
assert_no_symlink_path_components "production acceptance workspace" "$WORKSPACE"

if [ -e "$WORKSPACE" ] && [ ! -d "$WORKSPACE" ]; then
	fail "workspace path exists and is not a directory"
fi

if [ "$workspace_provided" -eq 1 ] && [ -d "$WORKSPACE" ]; then
	existing_entry="$(find "$WORKSPACE" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null || true)"
	if [ -n "$existing_entry" ] && [ "${OURO_PRODUCTION_ACCEPTANCE_SMOKE_ALLOW_EXISTING_WORKSPACE:-}" != "1" ]; then
		fail "refusing existing non-empty workspace; set OURO_PRODUCTION_ACCEPTANCE_SMOKE_ALLOW_EXISTING_WORKSPACE=1 only for a safe acceptance workspace"
	fi
fi

mkdir -p "$WORKSPACE"
assert_not_symlink_dir "production acceptance workspace" "$WORKSPACE"
assert_no_symlink_path_components "production acceptance workspace" "$WORKSPACE"

[ -n "${OURO_PRODUCTION_ACCEPTANCE_INVENTORY:-}" ] || fail "OURO_PRODUCTION_ACCEPTANCE_INVENTORY is required"
[ -n "${OURO_PRODUCTION_ACCEPTANCE_POLICY:-}" ] || fail "OURO_PRODUCTION_ACCEPTANCE_POLICY is required"
assert_absolute_path "inventory" "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY"
assert_absolute_path "deployment policy" "$OURO_PRODUCTION_ACCEPTANCE_POLICY"
assert_not_symlink_file "inventory" "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY"
assert_not_symlink_file "deployment policy" "$OURO_PRODUCTION_ACCEPTANCE_POLICY"
assert_no_symlink_path_components "inventory" "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY"
assert_no_symlink_path_components "deployment policy" "$OURO_PRODUCTION_ACCEPTANCE_POLICY"
[ -f "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" ] || fail "inventory file is missing: $OURO_PRODUCTION_ACCEPTANCE_INVENTORY"
[ -f "$OURO_PRODUCTION_ACCEPTANCE_POLICY" ] || fail "deployment policy file is missing: $OURO_PRODUCTION_ACCEPTANCE_POLICY"

read_field() {
	file="$1"
	key="$2"
	awk -v key="$key" '
		index($0, key "=") == 1 {
			print substr($0, length(key) + 2)
			found = 1
			exit
		}
		END {
			if (!found) {
				exit 1
			}
		}
	' "$file"
}

required_field() {
	file="$1"
	key="$2"
	value="$(read_field "$file" "$key")" || fail "missing required field $key in $file"
	[ -n "$value" ] || fail "required field $key is empty in $file"
	printf '%s\n' "$value"
}

validate_key_value_file() {
	file="$1"
	label="$2"
	validation_error="$(awk '
		/^[[:space:]]*$/ {
			next
		}
		{
			equals = index($0, "=")
			if (equals <= 1) {
				printf "malformed key=value line %d", NR
				exit 1
			}
			key = substr($0, 1, equals - 1)
			if (key !~ /^[A-Za-z_][A-Za-z0-9_]*$/) {
				printf "malformed key=value line %d", NR
				exit 1
			}
			if (key in seen) {
				printf "duplicate field %s", key
				exit 1
			}
			seen[key] = NR
		}
	' "$file")" || fail "$label invalid: $validation_error"
}

assert_field_equals() {
	file="$1"
	key="$2"
	expected="$3"
	actual="$(required_field "$file" "$key")"
	[ "$actual" = "$expected" ] || fail "$key expected $expected got $actual"
}

lowercase_sha256() {
	printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

assert_sha256() {
	label="$1"
	value="$2"
	lower="$(lowercase_sha256 "$value")"
	printf '%s\n' "$lower" | grep -E -q '^[0-9a-f]{64}$' || fail "$label must be a 64-character SHA-256 digest"
	printf '%s\n' "$lower"
}

assert_rfc3339_timestamp() {
	label="$1"
	value="$2"
	printf '%s\n' "$value" | grep -E -q '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$' || fail "$label must be RFC3339: $value"
}

file_sha256() {
	path="$1"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$path" | awk '{ print $1 }'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$path" | awk '{ print $1 }'
	else
		fail "sha256sum or shasum is required to write production acceptance audit digests"
	fi
}

validate_key_value_file "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" "inventory"
validate_key_value_file "$OURO_PRODUCTION_ACCEPTANCE_POLICY" "policy"
INVENTORY_SHA256="$(file_sha256 "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY")"
POLICY_SHA256="$(file_sha256 "$OURO_PRODUCTION_ACCEPTANCE_POLICY")"

assert_field_equals "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" schema ouroboros.production_acceptance.inventory.v1
assert_field_equals "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" inventory_status operator_owned
assert_field_equals "$OURO_PRODUCTION_ACCEPTANCE_POLICY" schema ouroboros.production_acceptance.policy.v1
assert_field_equals "$OURO_PRODUCTION_ACCEPTANCE_POLICY" policy_status approved
assert_field_equals "$OURO_PRODUCTION_ACCEPTANCE_POLICY" managed_payload_acceptance_required true
assert_field_equals "$OURO_PRODUCTION_ACCEPTANCE_POLICY" workflow_closure_required true
assert_field_equals "$OURO_PRODUCTION_ACCEPTANCE_POLICY" no_live_targets true
assert_field_equals "$OURO_PRODUCTION_ACCEPTANCE_POLICY" payload_execution_allowed false
assert_field_equals "$OURO_PRODUCTION_ACCEPTANCE_POLICY" network_download_allowed false
assert_field_equals "$OURO_PRODUCTION_ACCEPTANCE_POLICY" package_manager_allowed false
assert_field_equals "$OURO_PRODUCTION_ACCEPTANCE_POLICY" provider_calls_allowed false
assert_field_equals "$OURO_PRODUCTION_ACCEPTANCE_POLICY" worker_contact_allowed false

BINARY_NAME="$(required_field "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" binary_name)"
BINARY_PACK="$(required_field "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" binary_pack)"
BINARY_PAYLOAD="$(required_field "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" binary_payload)"
BINARY_EXPECTED_SHA256="$(assert_sha256 binary_expected_sha256 "$(required_field "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" binary_expected_sha256)")"
BINARY_VERSION="$(required_field "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" binary_version)"
BINARY_SOURCE="$(required_field "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" binary_source)"
RULE_NAME="$(required_field "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" rule_bundle_name)"
RULE_PACK="$(required_field "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" rule_bundle_pack)"
RULE_PAYLOAD="$(required_field "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" rule_bundle_payload)"
RULE_EXPECTED_SHA256="$(assert_sha256 rule_bundle_expected_sha256 "$(required_field "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" rule_bundle_expected_sha256)")"
RULE_VERSION="$(required_field "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" rule_bundle_version)"
RULE_SOURCE="$(required_field "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY" rule_bundle_source)"

DEPLOYMENT_ENVIRONMENT="$(required_field "$OURO_PRODUCTION_ACCEPTANCE_POLICY" deployment_environment)"
ACCEPTED_BY="$(required_field "$OURO_PRODUCTION_ACCEPTANCE_POLICY" accepted_by)"
ACCEPTED_AT="$(required_field "$OURO_PRODUCTION_ACCEPTANCE_POLICY" accepted_at)"
assert_rfc3339_timestamp accepted_at "$ACCEPTED_AT"
AUTHORIZATION_SCOPE="$(required_field "$OURO_PRODUCTION_ACCEPTANCE_POLICY" authorization_scope)"

assert_absolute_path "binary payload" "$BINARY_PAYLOAD"
assert_absolute_path "rule bundle payload" "$RULE_PAYLOAD"
assert_not_symlink_file "binary payload" "$BINARY_PAYLOAD"
assert_not_symlink_file "rule bundle payload" "$RULE_PAYLOAD"
assert_no_symlink_path_components "binary payload" "$BINARY_PAYLOAD"
assert_no_symlink_path_components "rule bundle payload" "$RULE_PAYLOAD"
[ -f "$BINARY_PAYLOAD" ] || fail "binary payload is not a regular file: $BINARY_PAYLOAD"
[ -d "$RULE_PAYLOAD" ] || fail "rule bundle payload is not a directory: $RULE_PAYLOAD"

RELEASE_WORKSPACE="$WORKSPACE/release-managed-workspace"
RELEASE_DIST="$WORKSPACE/release-managed-dist"
RELEASE_PREFIX="$WORKSPACE/release-managed-prefix"
RELEASE_LOG_DIR="$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/release-managed-logs"
IDE_WORKSPACE="$WORKSPACE/ide-smoke-workspace"
IDE_SMOKE_BIN_DIR="$WORKSPACE/ide-smoke-bin"
IDE_SMOKE_BIN="$IDE_SMOKE_BIN_DIR/ouro"
IDE_LOG_DIR="$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/ide-smoke-logs"
MANAGED_WORKSPACE="$WORKSPACE/managed-payloads-workspace"
WORKFLOW_WORKSPACE="$WORKSPACE/workflow-closure-workspace"
MANAGED_LOG_DIR="$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/managed-payloads-logs"
WORKFLOW_LOG_DIR="$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/workflow-closure-logs"
PRODUCTION_ACCEPTANCE_SCRIPT="$ROOT/scripts/smoke_production_acceptance.sh"
IDE_SMOKE_SCRIPT="$ROOT/scripts/smoke_ide.sh"
IDE_DIRECT_USE_SCRIPT="$ROOT/scripts/smoke_direct_use.sh"
IDE_AI_PROVIDER_SCRIPT="$ROOT/scripts/smoke_ai_provider.sh"
RELEASE_MANAGED_SCRIPT="$ROOT/scripts/smoke_release_managed.sh"
MANAGED_PAYLOADS_SCRIPT="$ROOT/scripts/smoke_managed_payloads.sh"
WORKFLOW_CLOSURE_SCRIPT="$ROOT/scripts/smoke_workflow_closure.sh"
IDE_DIRECT_USE_LOG="$IDE_LOG_DIR/direct-use.txt"
IDE_AI_PROVIDER_LOG="$IDE_LOG_DIR/ai-provider.txt"

mkdir -p "$WORKSPACE" "$IDE_LOG_DIR" "$RELEASE_LOG_DIR" "$MANAGED_LOG_DIR" "$WORKFLOW_LOG_DIR"
assert_output_file_path "ide smoke log" "$IDE_SMOKE_LOG"
assert_output_file_path "release managed log" "$RELEASE_MANAGED_LOG"
assert_output_file_path "managed payloads log" "$MANAGED_PAYLOADS_LOG"
assert_output_file_path "workflow closure log" "$WORKFLOW_CLOSURE_LOG"

cd "$ROOT"

PRODUCTION_ACCEPTANCE_SCRIPT_SHA256="$(file_sha256 "$PRODUCTION_ACCEPTANCE_SCRIPT")"
IDE_SMOKE_SCRIPT_SHA256="$(file_sha256 "$IDE_SMOKE_SCRIPT")"
IDE_DIRECT_USE_SCRIPT_SHA256="$(file_sha256 "$IDE_DIRECT_USE_SCRIPT")"
IDE_AI_PROVIDER_SCRIPT_SHA256="$(file_sha256 "$IDE_AI_PROVIDER_SCRIPT")"
RELEASE_MANAGED_SCRIPT_SHA256="$(file_sha256 "$RELEASE_MANAGED_SCRIPT")"
MANAGED_PAYLOADS_SCRIPT_SHA256="$(file_sha256 "$MANAGED_PAYLOADS_SCRIPT")"
WORKFLOW_CLOSURE_SCRIPT_SHA256="$(file_sha256 "$WORKFLOW_CLOSURE_SCRIPT")"

printf 'production-acceptance-smoke: workspace %s\n' "$WORKSPACE"
printf 'production-acceptance-smoke: inventory %s\n' "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY"
printf 'production-acceptance-smoke: policy %s\n' "$OURO_PRODUCTION_ACCEPTANCE_POLICY"
printf 'inventory: status=operator_owned binary=%s expected_sha256=%s\n' "$BINARY_NAME" "$BINARY_EXPECTED_SHA256"
printf 'inventory: status=operator_owned rule_bundle=%s expected_sha256=%s\n' "$RULE_NAME" "$RULE_EXPECTED_SHA256"
printf 'policy: status=approved environment=%s accepted_by=%s\n' "$DEPLOYMENT_ENVIRONMENT" "$ACCEPTED_BY"

IDE_SMOKE_LOG_TMP_PATH="$(mktemp "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/.ide-smoke.txt.XXXXXX")" || fail "unable to create IDE smoke log temporary file"
if ! {
	env \
		PATH="$TRUSTED_PATH" \
		GOPROXY="$GOPROXY" \
		BIN_DIR="$IDE_SMOKE_BIN_DIR" \
		BIN="$IDE_SMOKE_BIN" \
		"$TRUSTED_SH" "$ROOT/scripts/build.sh"
	env \
		PATH="$TRUSTED_PATH" \
		GOPROXY="$GOPROXY" \
		WORKSPACE="$IDE_WORKSPACE" \
		OURO_IDE_SMOKE_BIN="$IDE_SMOKE_BIN" \
		OURO_IDE_SMOKE_LOG_DIR="$IDE_LOG_DIR" \
		"$TRUSTED_SH" "$ROOT/scripts/smoke_ide.sh"
} > "$IDE_SMOKE_LOG_TMP_PATH" 2>&1; then
	assert_output_file_path "ide smoke log" "$IDE_SMOKE_LOG"
	mv -f "$IDE_SMOKE_LOG_TMP_PATH" "$IDE_SMOKE_LOG" || fail "unable to publish IDE smoke log"
	IDE_SMOKE_LOG_TMP_PATH=""
	IDE_SMOKE_LOG_PUBLISHED=1
	fail "IDE smoke acceptance failed; see $IDE_SMOKE_LOG"
fi
assert_output_file_path "ide smoke log" "$IDE_SMOKE_LOG"
mv -f "$IDE_SMOKE_LOG_TMP_PATH" "$IDE_SMOKE_LOG" || fail "unable to publish IDE smoke log"
IDE_SMOKE_LOG_TMP_PATH=""
IDE_SMOKE_LOG_PUBLISHED=1
grep -F -q 'ide-smoke: ok' "$IDE_SMOKE_LOG" || fail "IDE smoke acceptance did not report ok"
assert_not_symlink_file "IDE direct-use child log" "$IDE_DIRECT_USE_LOG"
assert_no_symlink_path_components "IDE direct-use child log" "$IDE_DIRECT_USE_LOG"
[ -f "$IDE_DIRECT_USE_LOG" ] || fail "IDE direct-use child log is missing: $IDE_DIRECT_USE_LOG"
grep -F -q 'direct-use-smoke: ok' "$IDE_DIRECT_USE_LOG" || fail "IDE direct-use child smoke did not report ok"
grep -F -q 'chat: available' "$IDE_DIRECT_USE_LOG" || fail "IDE direct-use child smoke did not prove chat availability"
assert_not_symlink_file "IDE AI-provider child log" "$IDE_AI_PROVIDER_LOG"
assert_no_symlink_path_components "IDE AI-provider child log" "$IDE_AI_PROVIDER_LOG"
[ -f "$IDE_AI_PROVIDER_LOG" ] || fail "IDE AI-provider child log is missing: $IDE_AI_PROVIDER_LOG"
grep -F -q 'ai-provider-smoke: ok' "$IDE_AI_PROVIDER_LOG" || fail "IDE AI-provider child smoke did not report ok"
grep -F -q 'chat-prompt: preview=created stored=false' "$IDE_AI_PROVIDER_LOG" || fail "IDE AI-provider child smoke did not prove direct chat prompt preview"
printf 'ide-smoke: ok\n'

RELEASE_MANAGED_LOG_TMP_PATH="$(mktemp "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/.release-managed.txt.XXXXXX")" || fail "unable to create release managed log temporary file"
if ! env \
		PATH="$TRUSTED_PATH" \
		GOPROXY="$GOPROXY" \
		WORKSPACE="$RELEASE_WORKSPACE" \
		DIST_DIR="$RELEASE_DIST" \
		PREFIX="$RELEASE_PREFIX" \
		VERSION="${OURO_PRODUCTION_ACCEPTANCE_RELEASE_VERSION:-0.1.0-production-acceptance-smoke}" \
		OURO_RELEASE_MANAGED_SMOKE_LOG_DIR="$RELEASE_LOG_DIR" \
		"$TRUSTED_SH" "$ROOT/scripts/smoke_release_managed.sh" > "$RELEASE_MANAGED_LOG_TMP_PATH" 2>&1; then
	assert_output_file_path "release managed log" "$RELEASE_MANAGED_LOG"
	mv -f "$RELEASE_MANAGED_LOG_TMP_PATH" "$RELEASE_MANAGED_LOG" || fail "unable to publish release managed log"
	RELEASE_MANAGED_LOG_TMP_PATH=""
	RELEASE_MANAGED_LOG_PUBLISHED=1
	fail "release-managed acceptance failed; see $RELEASE_MANAGED_LOG"
fi
assert_output_file_path "release managed log" "$RELEASE_MANAGED_LOG"
mv -f "$RELEASE_MANAGED_LOG_TMP_PATH" "$RELEASE_MANAGED_LOG" || fail "unable to publish release managed log"
RELEASE_MANAGED_LOG_TMP_PATH=""
RELEASE_MANAGED_LOG_PUBLISHED=1
grep -F -q 'release-managed-smoke: ok' "$RELEASE_MANAGED_LOG" || fail "release-managed acceptance did not report ok"
printf 'release-managed: ok\n'

MANAGED_PAYLOADS_LOG_TMP_PATH="$(mktemp "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/.managed-payloads.txt.XXXXXX")" || fail "unable to create managed payloads log temporary file"
if ! env \
		PATH="$TRUSTED_PATH" \
		GOPROXY="$GOPROXY" \
		WORKSPACE="$MANAGED_WORKSPACE" \
	OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR="$MANAGED_LOG_DIR" \
	OURO_MANAGED_BINARY_NAME="$BINARY_NAME" \
	OURO_MANAGED_BINARY_PACK="$BINARY_PACK" \
	OURO_MANAGED_BINARY_PAYLOAD="$BINARY_PAYLOAD" \
	OURO_MANAGED_BINARY_EXPECTED_SHA256="$BINARY_EXPECTED_SHA256" \
	OURO_MANAGED_BINARY_VERSION="$BINARY_VERSION" \
	OURO_MANAGED_BINARY_SOURCE="$BINARY_SOURCE" \
	OURO_MANAGED_RULE_BUNDLE_NAME="$RULE_NAME" \
	OURO_MANAGED_RULE_BUNDLE_PACK="$RULE_PACK" \
		OURO_MANAGED_RULE_BUNDLE_PAYLOAD="$RULE_PAYLOAD" \
		OURO_MANAGED_RULE_BUNDLE_EXPECTED_SHA256="$RULE_EXPECTED_SHA256" \
		OURO_MANAGED_RULE_BUNDLE_VERSION="$RULE_VERSION" \
		OURO_MANAGED_RULE_BUNDLE_SOURCE="$RULE_SOURCE" \
		"$TRUSTED_SH" "$ROOT/scripts/smoke_managed_payloads.sh" > "$MANAGED_PAYLOADS_LOG_TMP_PATH" 2>&1; then
	assert_output_file_path "managed payloads log" "$MANAGED_PAYLOADS_LOG"
	mv -f "$MANAGED_PAYLOADS_LOG_TMP_PATH" "$MANAGED_PAYLOADS_LOG" || fail "unable to publish managed payloads log"
	MANAGED_PAYLOADS_LOG_TMP_PATH=""
	MANAGED_PAYLOADS_LOG_PUBLISHED=1
	fail "managed payload acceptance failed; see $MANAGED_PAYLOADS_LOG"
fi
assert_output_file_path "managed payloads log" "$MANAGED_PAYLOADS_LOG"
mv -f "$MANAGED_PAYLOADS_LOG_TMP_PATH" "$MANAGED_PAYLOADS_LOG" || fail "unable to publish managed payloads log"
MANAGED_PAYLOADS_LOG_TMP_PATH=""
MANAGED_PAYLOADS_LOG_PUBLISHED=1
grep -F -q 'managed-payloads-smoke: ok' "$MANAGED_PAYLOADS_LOG" || fail "managed payload acceptance did not report ok"
printf 'managed-payloads: ok\n'

WORKFLOW_CLOSURE_LOG_TMP_PATH="$(mktemp "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/.workflow-closure.txt.XXXXXX")" || fail "unable to create workflow closure log temporary file"
if ! env \
		PATH="$TRUSTED_PATH" \
		GOPROXY="$GOPROXY" \
		WORKSPACE="$WORKFLOW_WORKSPACE" \
		OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR="$WORKFLOW_LOG_DIR" \
		"$TRUSTED_SH" "$ROOT/scripts/smoke_workflow_closure.sh" > "$WORKFLOW_CLOSURE_LOG_TMP_PATH" 2>&1; then
	assert_output_file_path "workflow closure log" "$WORKFLOW_CLOSURE_LOG"
	mv -f "$WORKFLOW_CLOSURE_LOG_TMP_PATH" "$WORKFLOW_CLOSURE_LOG" || fail "unable to publish workflow closure log"
	WORKFLOW_CLOSURE_LOG_TMP_PATH=""
	WORKFLOW_CLOSURE_LOG_PUBLISHED=1
	fail "workflow closure acceptance failed; see $WORKFLOW_CLOSURE_LOG"
fi
assert_output_file_path "workflow closure log" "$WORKFLOW_CLOSURE_LOG"
mv -f "$WORKFLOW_CLOSURE_LOG_TMP_PATH" "$WORKFLOW_CLOSURE_LOG" || fail "unable to publish workflow closure log"
WORKFLOW_CLOSURE_LOG_TMP_PATH=""
WORKFLOW_CLOSURE_LOG_PUBLISHED=1
grep -F -q 'workflow-closure-smoke: ok' "$WORKFLOW_CLOSURE_LOG" || fail "workflow closure acceptance did not report ok"
printf 'workflow-closure: ok\n'

IDE_LOG_SHA256="$(file_sha256 "$IDE_SMOKE_LOG")"
IDE_DIRECT_USE_LOG_SHA256="$(file_sha256 "$IDE_DIRECT_USE_LOG")"
IDE_AI_PROVIDER_LOG_SHA256="$(file_sha256 "$IDE_AI_PROVIDER_LOG")"
RELEASE_LOG_SHA256="$(file_sha256 "$RELEASE_MANAGED_LOG")"
MANAGED_LOG_SHA256="$(file_sha256 "$MANAGED_PAYLOADS_LOG")"
WORKFLOW_LOG_SHA256="$(file_sha256 "$WORKFLOW_CLOSURE_LOG")"
COMPLETED_AT="$(timestamp_utc)"

assert_output_file_path "production acceptance summary" "$SUMMARY_PATH"
SUMMARY_TMP_PATH="$(mktemp "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/.summary.txt.XXXXXX")" || fail "unable to create production acceptance summary temporary file"
if {
	printf 'schema=ouroboros.production_acceptance.audit.v1\n'
	printf 'audit_id=%s\n' "$AUDIT_ID"
	printf 'started_at=%s\n' "$STARTED_AT"
	printf 'completed_at=%s\n' "$COMPLETED_AT"
	printf 'status=accepted\n'
	printf 'mode=operator-owned\n'
	printf 'inventory=%s\n' "$OURO_PRODUCTION_ACCEPTANCE_INVENTORY"
	printf 'policy=%s\n' "$OURO_PRODUCTION_ACCEPTANCE_POLICY"
	printf 'inventory_sha256=%s\n' "$INVENTORY_SHA256"
	printf 'policy_sha256=%s\n' "$POLICY_SHA256"
	printf 'workspace=%s\n' "$WORKSPACE"
	printf 'ide_smoke_workspace=%s\n' "$IDE_WORKSPACE"
	printf 'release_managed_workspace=%s\n' "$RELEASE_WORKSPACE"
	printf 'managed_payloads_workspace=%s\n' "$MANAGED_WORKSPACE"
	printf 'workflow_closure_workspace=%s\n' "$WORKFLOW_WORKSPACE"
	printf 'trusted_shell=%s\n' "$TRUSTED_SH"
	printf 'trusted_path=%s\n' "$TRUSTED_PATH"
	printf 'production_acceptance_script=%s\n' "$PRODUCTION_ACCEPTANCE_SCRIPT"
	printf 'production_acceptance_script_sha256=%s\n' "$PRODUCTION_ACCEPTANCE_SCRIPT_SHA256"
	printf 'ide_smoke_script=%s\n' "$IDE_SMOKE_SCRIPT"
	printf 'ide_smoke_script_sha256=%s\n' "$IDE_SMOKE_SCRIPT_SHA256"
	printf 'ide_direct_use_script=%s\n' "$IDE_DIRECT_USE_SCRIPT"
	printf 'ide_direct_use_script_sha256=%s\n' "$IDE_DIRECT_USE_SCRIPT_SHA256"
	printf 'ide_ai_provider_script=%s\n' "$IDE_AI_PROVIDER_SCRIPT"
	printf 'ide_ai_provider_script_sha256=%s\n' "$IDE_AI_PROVIDER_SCRIPT_SHA256"
	printf 'release_managed_script=%s\n' "$RELEASE_MANAGED_SCRIPT"
	printf 'release_managed_script_sha256=%s\n' "$RELEASE_MANAGED_SCRIPT_SHA256"
	printf 'managed_payloads_script=%s\n' "$MANAGED_PAYLOADS_SCRIPT"
	printf 'managed_payloads_script_sha256=%s\n' "$MANAGED_PAYLOADS_SCRIPT_SHA256"
	printf 'workflow_closure_script=%s\n' "$WORKFLOW_CLOSURE_SCRIPT"
	printf 'workflow_closure_script_sha256=%s\n' "$WORKFLOW_CLOSURE_SCRIPT_SHA256"
	printf 'binary_name=%s\n' "$BINARY_NAME"
	printf 'binary_expected_sha256=%s\n' "$BINARY_EXPECTED_SHA256"
	printf 'rule_bundle_name=%s\n' "$RULE_NAME"
	printf 'rule_bundle_expected_sha256=%s\n' "$RULE_EXPECTED_SHA256"
	printf 'deployment_environment=%s\n' "$DEPLOYMENT_ENVIRONMENT"
	printf 'accepted_by=%s\n' "$ACCEPTED_BY"
	printf 'accepted_at=%s\n' "$ACCEPTED_AT"
	printf 'authorization_scope=%s\n' "$AUTHORIZATION_SCOPE"
	printf 'ide_smoke_acceptance=passed\n'
	printf 'release_managed_acceptance=passed\n'
	printf 'managed_payload_acceptance=passed\n'
	printf 'workflow_closure_acceptance=passed\n'
	printf 'ide_smoke_log=%s\n' "$IDE_SMOKE_LOG"
	printf 'ide_direct_use_log=%s\n' "$IDE_DIRECT_USE_LOG"
	printf 'ide_ai_provider_log=%s\n' "$IDE_AI_PROVIDER_LOG"
	printf 'release_managed_log=%s\n' "$RELEASE_MANAGED_LOG"
	printf 'managed_payloads_log=%s\n' "$MANAGED_PAYLOADS_LOG"
	printf 'workflow_closure_log=%s\n' "$WORKFLOW_CLOSURE_LOG"
	printf 'ide_smoke_log_sha256=%s\n' "$IDE_LOG_SHA256"
	printf 'ide_direct_use_log_sha256=%s\n' "$IDE_DIRECT_USE_LOG_SHA256"
	printf 'ide_ai_provider_log_sha256=%s\n' "$IDE_AI_PROVIDER_LOG_SHA256"
	printf 'release_managed_log_sha256=%s\n' "$RELEASE_LOG_SHA256"
	printf 'managed_payloads_log_sha256=%s\n' "$MANAGED_LOG_SHA256"
	printf 'workflow_closure_log_sha256=%s\n' "$WORKFLOW_LOG_SHA256"
	printf 'no_live_targets=true\n'
	printf 'payload_execution_allowed=false\n'
	printf 'network_download_allowed=false\n'
	printf 'package_manager_allowed=false\n'
	printf 'provider_calls_allowed=false\n'
	printf 'worker_contact_allowed=false\n'
} > "$SUMMARY_TMP_PATH"; then
	mv -f "$SUMMARY_TMP_PATH" "$SUMMARY_PATH" || fail "unable to publish production acceptance summary"
	SUMMARY_TMP_PATH=""
else
	fail "unable to write production acceptance summary"
fi

printf 'production-acceptance: status=accepted mode=operator-owned\n'
printf 'production-acceptance-smoke: ok\n'
