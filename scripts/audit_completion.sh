#!/bin/sh
set -eu

TRUSTED_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH="$TRUSTED_PATH"
export PATH

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
TMPDIR_BASE="${TMPDIR:-/tmp}"
COMPLETION_AUDIT_SCRIPT="$ROOT/scripts/audit_completion.sh"

COMPLETION_AUDIT_LOG_DIR_CONFIGURED=1
if [ -z "${OURO_COMPLETION_AUDIT_LOG_DIR:-}" ]; then
	COMPLETION_AUDIT_LOG_DIR_CONFIGURED=0
	OURO_COMPLETION_AUDIT_LOG_DIR="$(mktemp -d "$TMPDIR_BASE/ouro-completion-audit-logs-XXXXXX")"
fi

RESULT_PATH="$OURO_COMPLETION_AUDIT_LOG_DIR/completion-audit.txt"
MANIFEST_PATH="$OURO_COMPLETION_AUDIT_LOG_DIR/completion-evidence-manifest.txt"
MANIFEST_DIGEST_PATH="$OURO_COMPLETION_AUDIT_LOG_DIR/completion-evidence-manifest.sha256"
OUTPUT_PATHS_VERIFIED=0
STALE_OUTPUT_CLEANUP_VERIFIED=0
RESULT_TMP_PATH=""
MANIFEST_TMP_PATH=""
MANIFEST_DIGEST_TMP_PATH=""

cleanup_completion_audit_output_temps() {
	if [ -n "$RESULT_TMP_PATH" ]; then
		rm -f "$RESULT_TMP_PATH" 2>/dev/null || true
	fi
	if [ -n "$MANIFEST_TMP_PATH" ]; then
		rm -f "$MANIFEST_TMP_PATH" 2>/dev/null || true
	fi
	if [ -n "$MANIFEST_DIGEST_TMP_PATH" ]; then
		rm -f "$MANIFEST_DIGEST_TMP_PATH" 2>/dev/null || true
	fi
}

trap cleanup_completion_audit_output_temps EXIT HUP INT TERM

timestamp_utc() {
	date -u '+%Y-%m-%dT%H:%M:%SZ'
}

STARTED_AT="$(timestamp_utc)"
AUDIT_ID="${OURO_COMPLETION_AUDIT_ID:-completion-audit-$(date -u '+%Y%m%dT%H%M%SZ')-$$}"

remove_stale_completion_evidence_outputs() {
	[ "$STALE_OUTPUT_CLEANUP_VERIFIED" = "1" ] || return 0
	if [ ! -L "$MANIFEST_PATH" ] && [ -f "$MANIFEST_PATH" ]; then
		rm -f "$MANIFEST_PATH" 2>/dev/null || true
	fi
	if [ ! -L "$MANIFEST_DIGEST_PATH" ] && [ -f "$MANIFEST_DIGEST_PATH" ]; then
		rm -f "$MANIFEST_DIGEST_PATH" 2>/dev/null || true
	fi
}

write_result() {
	status="$1"
	reason="$2"
	[ "$OUTPUT_PATHS_VERIFIED" = "1" ] || return 0
	[ ! -L "$RESULT_PATH" ] || return 0
	if [ -e "$RESULT_PATH" ] && [ ! -f "$RESULT_PATH" ]; then
		return 0
	fi
	result_tmp="$(mktemp "$OURO_COMPLETION_AUDIT_LOG_DIR/.completion-audit.txt.XXXXXX" 2>/dev/null)" || return 0
	if {
		printf 'schema=ouroboros.completion_audit.result.v1\n'
		printf 'audit_id=%s\n' "$AUDIT_ID"
		printf 'started_at=%s\n' "$STARTED_AT"
		printf 'completed_at=%s\n' "$(timestamp_utc)"
		printf 'status=%s\n' "$status"
		if [ -n "$reason" ]; then
			printf 'failure_reason=%s\n' "$(printf '%s' "$reason" | tr '\n' ' ')"
		fi
	} > "$result_tmp" 2>/dev/null; then
		if mv -f "$result_tmp" "$RESULT_PATH" 2>/dev/null; then
			if [ "$status" = "failed" ]; then
				remove_stale_completion_evidence_outputs
			fi
		else
			rm -f "$result_tmp" 2>/dev/null || true
		fi
	else
		rm -f "$result_tmp" 2>/dev/null || true
	fi
}

fail() {
	remove_stale_completion_evidence_outputs
	write_result failed "$1"
	printf 'completion-audit: failed: %s\n' "$1" >&2
	printf 'completion-audit: logs %s\n' "$OURO_COMPLETION_AUDIT_LOG_DIR" >&2
	exit 1
}

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

assert_output_file_path() {
	label="$1"
	path="$2"
	assert_not_symlink_file "$label" "$path"
	if [ -e "$path" ] && [ ! -f "$path" ]; then
		fail "$label file must be a regular file or absent: $path"
	fi
}

assert_absolute_path() {
	label="$1"
	path="$2"
	case "$path" in
		/*) ;;
		*) fail "$label path must be absolute: $path" ;;
	esac
}

assert_absolute_path "completion audit log dir" "$OURO_COMPLETION_AUDIT_LOG_DIR"
assert_not_symlink_dir "completion audit log dir" "$OURO_COMPLETION_AUDIT_LOG_DIR"
if [ "$COMPLETION_AUDIT_LOG_DIR_CONFIGURED" = "1" ]; then
	assert_no_symlink_path_components "completion audit log dir" "$OURO_COMPLETION_AUDIT_LOG_DIR"
fi
mkdir -p "$OURO_COMPLETION_AUDIT_LOG_DIR"
assert_not_symlink_dir "completion audit log dir" "$OURO_COMPLETION_AUDIT_LOG_DIR"
if [ "$COMPLETION_AUDIT_LOG_DIR_CONFIGURED" = "1" ]; then
	assert_no_symlink_path_components "completion audit log dir" "$OURO_COMPLETION_AUDIT_LOG_DIR"
fi
STALE_OUTPUT_CLEANUP_VERIFIED=1
assert_output_file_path "completion audit result" "$RESULT_PATH"
OUTPUT_PATHS_VERIFIED=1
assert_output_file_path "completion audit manifest" "$MANIFEST_PATH"
assert_output_file_path "completion audit manifest digest" "$MANIFEST_DIGEST_PATH"

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

assert_sha256() {
	label="$1"
	value="$2"
	printf '%s\n' "$value" | grep -E -q '^[0-9a-fA-F]{64}$' || fail "$label must be a 64-character SHA-256 digest"
	printf '%s\n' "$value" | tr '[:upper:]' '[:lower:]'
}

assert_rfc3339_timestamp() {
	label="$1"
	value="$2"
	printf '%s\n' "$value" | grep -E -q '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$' || fail "$label must be RFC3339: $value"
}

timestamp_epoch() {
	label="$1"
	value="$2"
	epoch="$(date -u -d "$value" '+%s' 2>/dev/null)" || fail "$label must be RFC3339: $value"
	digits="$epoch"
	case "$digits" in
		-*) digits="${digits#-}" ;;
	esac
	case "$digits" in
		""|*[!0-9]*) fail "$label must be RFC3339: $value" ;;
	esac
	printf '%s\n' "$epoch"
}

assert_timestamp_not_before() {
	start_label="$1"
	start_value="$2"
	end_label="$3"
	end_value="$4"
	start_epoch="$(timestamp_epoch "$start_label" "$start_value")"
	end_epoch="$(timestamp_epoch "$end_label" "$end_value")"
	[ "$end_epoch" -ge "$start_epoch" ] || fail "$end_label must not be before $start_label"
}

assert_timestamp_not_after() {
	value_label="$1"
	value="$2"
	end_label="$3"
	end_value="$4"
	value_epoch="$(timestamp_epoch "$value_label" "$value")"
	end_epoch="$(timestamp_epoch "$end_label" "$end_value")"
	[ "$value_epoch" -le "$end_epoch" ] || fail "$value_label must not be after $end_label"
}

file_sha256() {
	path="$1"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$path" | awk '{ print $1 }'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$path" | awk '{ print $1 }'
	else
		fail "sha256sum or shasum is required to write completion audit digests"
	fi
}

append_byte_to_file() {
	append_byte_output="$1"
	append_byte_value="$2"
	append_byte_octet="$(printf '%03o' "$append_byte_value")"
	printf '%b' "\\$append_byte_octet" >> "$append_byte_output"
}

append_uint64_be_to_file() {
	append_uint_output="$1"
	append_uint_value="$2"
	for append_uint_divisor in 72057594037927936 281474976710656 1099511627776 4294967296 16777216 65536 256 1; do
		append_byte_to_file "$append_uint_output" "$((append_uint_value / append_uint_divisor % 256))"
	done
}

append_sha256_hex_to_file() {
	append_sha_output="$1"
	append_sha_hex="$2"
	while [ -n "$append_sha_hex" ]; do
		append_sha_pair="$(printf '%s' "$append_sha_hex" | cut -c 1-2)"
		append_sha_hex="$(printf '%s' "$append_sha_hex" | cut -c 3-)"
		append_byte_to_file "$append_sha_output" "$(printf '%d' "0x$append_sha_pair")"
	done
}

directory_sha256() {
	label="$1"
	dir="$2"
	assert_absolute_path "$label" "$dir"
	assert_not_symlink_dir "$label" "$dir"
	assert_no_symlink_path_components "$label" "$dir"
	[ -d "$dir" ] || fail "$label directory is missing: $dir"

	non_regular="$(find "$dir" ! -type d ! -type f -print -quit 2>/dev/null)" || fail "$label directory inspection failed: $dir"
	[ -z "$non_regular" ] || fail "$label directory must contain only regular files: $non_regular"

	list_tmp="$(mktemp "$OURO_COMPLETION_AUDIT_LOG_DIR/.rule-bundle-files.XXXXXX")" || fail "unable to create $label file list temporary file"
	record_tmp="$(mktemp "$OURO_COMPLETION_AUDIT_LOG_DIR/.rule-bundle-digest.XXXXXX")" || {
		rm -f "$list_tmp" 2>/dev/null || true
		fail "unable to create $label digest temporary file"
	}
	if ! find "$dir" -type f -print 2>/dev/null | LC_ALL=C sort > "$list_tmp"; then
		rm -f "$list_tmp" "$record_tmp" 2>/dev/null || true
		fail "$label directory listing failed: $dir"
	fi
	if [ ! -s "$list_tmp" ]; then
		rm -f "$list_tmp" "$record_tmp" 2>/dev/null || true
		fail "$label directory must contain at least one regular file: $dir"
	fi

	printf 'ouroboros managed rule_bundle directory digest v1\n' > "$record_tmp"
	while IFS= read -r file_path; do
		assert_not_symlink_file "$label digest file" "$file_path"
		[ -f "$file_path" ] || {
			rm -f "$list_tmp" "$record_tmp" 2>/dev/null || true
			fail "$label digest file must be a regular file: $file_path"
		}
		case "$file_path" in
			"$dir"/*) rel_path="${file_path#"$dir"/}" ;;
			*)
				rm -f "$list_tmp" "$record_tmp" 2>/dev/null || true
				fail "$label digest file is outside directory: $file_path"
				;;
		esac
		rel_len="$(printf '%s' "$rel_path" | wc -c | awk '{ print $1 }')"
		file_size="$(wc -c < "$file_path" | awk '{ print $1 }')"
		content_sha="$(file_sha256 "$file_path")"
		append_uint64_be_to_file "$record_tmp" "$rel_len"
		printf '%s' "$rel_path" >> "$record_tmp"
		append_uint64_be_to_file "$record_tmp" "$file_size"
		append_sha256_hex_to_file "$record_tmp" "$content_sha"
	done < "$list_tmp"

	digest="$(file_sha256 "$record_tmp")"
	rm -f "$list_tmp" "$record_tmp" 2>/dev/null || true
	printf '%s\n' "$digest"
}

assert_file_sha256() {
	label="$1"
	path="$2"
	expected="$3"
	assert_absolute_path "$label" "$path"
	assert_not_symlink_file "$label" "$path"
	assert_no_symlink_path_components "$label" "$path"
	[ -f "$path" ] || fail "$label file is missing: $path"
	actual="$(file_sha256 "$path")"
	[ "$actual" = "$expected" ] || fail "$label mismatch expected $expected got $actual"
}

assert_file_contains_marker() {
	label="$1"
	path="$2"
	marker="$3"
	grep -F -q "$marker" "$path" || fail "$label missing required marker: $marker"
}

assert_summary_stage_log_path() {
	label="$1"
	path="$2"
	file_name="$3"
	expected="$(dirname -- "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY")/$file_name"
	[ "$path" = "$expected" ] || fail "$label must reference expected production acceptance log $expected got $path"
}

assert_file_contains_regex_marker() {
	label="$1"
	path="$2"
	marker="$3"
	regex="$4"
	grep -E -q "$regex" "$path" || fail "$label missing required marker: $marker"
}

assert_json_status_values_current() {
	label="$1"
	path="$2"
	status_values="$(awk '
		{
			line = $0
			while (match(line, /"status"[[:space:]]*:[[:space:]]*"[^"]*"/)) {
				token = substr(line, RSTART, RLENGTH)
				sub(/^.*"status"[[:space:]]*:[[:space:]]*"/, "", token)
				sub(/"$/, "", token)
				print token
				line = substr(line, RSTART + RLENGTH)
			}
		}
	' "$path")" || fail "$label status inspection failed: $path"
	[ -n "$status_values" ] || fail "$label status is missing"
	bad_status="$(printf '%s\n' "$status_values" | awk '$0 != "current" { print; exit }')"
	[ -z "$bad_status" ] || fail "$label status must be current: $bad_status"
}

json_string_field() {
	label="$1"
	path="$2"
	key="$3"
	value="$(awk -v key="$key" '
		{
			line = $0
			while (match(line, "\"" key "\"[[:space:]]*:[[:space:]]*\"[^\"]*\"")) {
				token = substr(line, RSTART, RLENGTH)
				sub("^.*\"" key "\"[[:space:]]*:[[:space:]]*\"", "", token)
				sub(/"$/, "", token)
				print token
				found = 1
				exit
			}
		}
		END {
			if (!found) {
				exit 1
			}
		}
	' "$path")" || fail "$label $key is missing"
	[ -n "$value" ] || fail "$label $key is empty"
	printf '%s\n' "$value"
}

json_bool_field() {
	label="$1"
	path="$2"
	key="$3"
	value="$(awk -v key="$key" '
		{
			line = $0
			while (match(line, "\"" key "\"[[:space:]]*:[[:space:]]*(true|false)")) {
				token = substr(line, RSTART, RLENGTH)
				sub("^.*\"" key "\"[[:space:]]*:[[:space:]]*", "", token)
				print token
				found = 1
				exit
			}
		}
		END {
			if (!found) {
				exit 1
			}
		}
	' "$path")" || fail "$label $key is missing"
	printf '%s\n' "$value"
}

json_int_field() {
	label="$1"
	path="$2"
	key="$3"
	value="$(awk -v key="$key" '
		{
			line = $0
			while (match(line, "\"" key "\"[[:space:]]*:[[:space:]]*[0-9]+")) {
				token = substr(line, RSTART, RLENGTH)
				sub("^.*\"" key "\"[[:space:]]*:[[:space:]]*", "", token)
				print token
				found = 1
				exit
			}
		}
		END {
			if (!found) {
				exit 1
			}
		}
	' "$path")" || fail "$label $key is missing"
	printf '%s\n' "$value"
}

assert_json_string_field_equals() {
	label="$1"
	path="$2"
	key="$3"
	expected="$4"
	actual="$(json_string_field "$label" "$path" "$key")"
	[ "$actual" = "$expected" ] || fail "$label $key expected $expected got $actual"
}

assert_json_bool_field_equals() {
	label="$1"
	path="$2"
	key="$3"
	expected="$4"
	actual="$(json_bool_field "$label" "$path" "$key")"
	[ "$actual" = "$expected" ] || fail "$label $key expected $expected got $actual"
}

assert_json_int_field_equals() {
	label="$1"
	path="$2"
	key="$3"
	expected="$4"
	actual="$(json_int_field "$label" "$path" "$key")"
	[ "$actual" = "$expected" ] || fail "$label $key expected $expected got $actual"
}

assert_safe_basename() {
	label="$1"
	value="$2"
	[ -n "$value" ] || fail "$label must not be empty"
	case "$value" in
		.|..|*/*|*\\*) fail "$label must be a safe basename: $value" ;;
	esac
}

assert_direct_json_file_under() {
	label="$1"
	path="$2"
	dir="$3"
	assert_absolute_path "$label" "$path"
	assert_no_symlink_path_components "$label" "$path"
	case "$path" in
		"$dir"/*.json) ;;
		*) fail "$label must be a direct json file under $dir: $path" ;;
	esac
	base="$(basename -- "$path")"
	case "$base" in
		*.json) ;;
		*) fail "$label must be a json file: $path" ;;
	esac
	[ -f "$path" ] || fail "$label file is missing: $path"
}

assert_workflow_report_selection_provenance() {
	workflow_selection_label="$1"
	workflow_selection_path="$2"
	workflow_selection_workspace="$3"
	workflow_selection_started_at="$4"
	workflow_selection_completed_at="$5"
	assert_json_string_field_equals "$workflow_selection_label" "$workflow_selection_path" mode metadata-only
	workflow_selection_execution_path="$(json_string_field "$workflow_selection_label" "$workflow_selection_path" execution_path)"
	workflow_selection_dispatch_path="$(json_string_field "$workflow_selection_label" "$workflow_selection_path" dispatch_path)"
	workflow_selection_run_id="$(json_string_field "$workflow_selection_label" "$workflow_selection_path" run_id)"
	workflow_selection_report_path="$(json_string_field "$workflow_selection_label" "$workflow_selection_path" report_path)"
	workflow_selection_event_count="$(json_int_field "$workflow_selection_label" "$workflow_selection_path" event_count)"
	assert_json_bool_field_equals "$workflow_selection_label" "$workflow_selection_path" written true
	workflow_selection_selected_by="$(json_string_field "$workflow_selection_label" "$workflow_selection_path" selected_by)"
	[ -n "$workflow_selection_selected_by" ] || fail "$workflow_selection_label selected_by is empty"
	workflow_selection_selected_at="$(json_string_field "$workflow_selection_label" "$workflow_selection_path" selected_at)"
	assert_rfc3339_timestamp "$workflow_selection_label selected_at" "$workflow_selection_selected_at"
	assert_timestamp_not_before started_at "$workflow_selection_started_at" "$workflow_selection_label selected_at" "$workflow_selection_selected_at"
	assert_timestamp_not_after "$workflow_selection_label selected_at" "$workflow_selection_selected_at" completed_at "$workflow_selection_completed_at"
	assert_safe_basename "$workflow_selection_label run_id" "$workflow_selection_run_id"
	workflow_selection_expected_report_path="$workflow_selection_workspace/reports/$workflow_selection_run_id.md"
	[ "$workflow_selection_report_path" = "$workflow_selection_expected_report_path" ] || fail "$workflow_selection_label report_path expected $workflow_selection_expected_report_path got $workflow_selection_report_path"
	assert_stage_workspace_file "$workflow_selection_label report" "$workflow_selection_report_path"
	workflow_selection_report_sha256="$(file_sha256 "$workflow_selection_report_path")"
	workflow_selection_execution_dir="$workflow_selection_workspace/reports/report-projection-dispatch-executions"
	workflow_selection_dispatch_dir="$workflow_selection_workspace/reports/report-projection-dispatches"
	assert_direct_json_file_under "$workflow_selection_label execution_path" "$workflow_selection_execution_path" "$workflow_selection_execution_dir"
	assert_direct_json_file_under "$workflow_selection_label dispatch_path" "$workflow_selection_dispatch_path" "$workflow_selection_dispatch_dir"
	workflow_selection_execution_sha256="$(file_sha256 "$workflow_selection_execution_path")"
	workflow_selection_dispatch_sha256="$(file_sha256 "$workflow_selection_dispatch_path")"

	workflow_dispatch_label="$workflow_selection_label dispatch"
	assert_json_string_field_equals "$workflow_dispatch_label" "$workflow_selection_dispatch_path" status dry-run
	assert_json_string_field_equals "$workflow_dispatch_label" "$workflow_selection_dispatch_path" mode metadata-only
	workflow_dispatch_created_at="$(json_string_field "$workflow_dispatch_label" "$workflow_selection_dispatch_path" created_at)"
	assert_rfc3339_timestamp "$workflow_dispatch_label created_at" "$workflow_dispatch_created_at"
	assert_timestamp_not_before started_at "$workflow_selection_started_at" "$workflow_dispatch_label created_at" "$workflow_dispatch_created_at"
	assert_timestamp_not_after "$workflow_dispatch_label created_at" "$workflow_dispatch_created_at" selected_at "$workflow_selection_selected_at"
	assert_json_string_field_equals "$workflow_dispatch_label" "$workflow_selection_dispatch_path" run_id "$workflow_selection_run_id"
	assert_json_string_field_equals "$workflow_dispatch_label" "$workflow_selection_dispatch_path" report_path "$workflow_selection_report_path"
	workflow_dispatch_event_count="$(json_int_field "$workflow_dispatch_label" "$workflow_selection_dispatch_path" event_count)"
	if [ "$workflow_dispatch_event_count" -gt "$workflow_selection_event_count" ]; then
		fail "$workflow_dispatch_label event_count must not exceed selection event_count"
	fi

	workflow_execution_label="$workflow_selection_label execution"
	assert_json_string_field_equals "$workflow_execution_label" "$workflow_selection_execution_path" status completed
	assert_json_string_field_equals "$workflow_execution_label" "$workflow_selection_execution_path" mode execute
	workflow_execution_executed_at="$(json_string_field "$workflow_execution_label" "$workflow_selection_execution_path" executed_at)"
	assert_rfc3339_timestamp "$workflow_execution_label executed_at" "$workflow_execution_executed_at"
	assert_timestamp_not_before "$workflow_dispatch_label created_at" "$workflow_dispatch_created_at" "$workflow_execution_label executed_at" "$workflow_execution_executed_at"
	assert_timestamp_not_after "$workflow_execution_label executed_at" "$workflow_execution_executed_at" selected_at "$workflow_selection_selected_at"
	assert_json_string_field_equals "$workflow_execution_label" "$workflow_selection_execution_path" dispatch_path "$workflow_selection_dispatch_path"
	assert_json_string_field_equals "$workflow_execution_label" "$workflow_selection_execution_path" run_id "$workflow_selection_run_id"
	assert_json_string_field_equals "$workflow_execution_label" "$workflow_selection_execution_path" report_path "$workflow_selection_report_path"
	workflow_execution_report_sha256="$(assert_sha256 "$workflow_execution_label report_sha256" "$(json_string_field "$workflow_execution_label" "$workflow_selection_execution_path" report_sha256)")"
	[ "$workflow_execution_report_sha256" = "$workflow_selection_report_sha256" ] || fail "$workflow_execution_label report_sha256 mismatch expected $workflow_execution_report_sha256 got $workflow_selection_report_sha256"
	assert_json_int_field_equals "$workflow_execution_label" "$workflow_selection_execution_path" event_count "$workflow_selection_event_count"
	assert_json_bool_field_equals "$workflow_execution_label" "$workflow_selection_execution_path" written true
	assert_json_string_field_equals "$workflow_execution_label annotation" "$workflow_selection_execution_path" decision accepted
	workflow_execution_reviewed_at="$(json_string_field "$workflow_execution_label annotation" "$workflow_selection_execution_path" reviewed_at)"
	assert_rfc3339_timestamp "$workflow_execution_label annotation reviewed_at" "$workflow_execution_reviewed_at"
	assert_timestamp_not_before "$workflow_execution_label executed_at" "$workflow_execution_executed_at" "$workflow_execution_label annotation reviewed_at" "$workflow_execution_reviewed_at"
	assert_timestamp_not_after "$workflow_execution_label annotation reviewed_at" "$workflow_execution_reviewed_at" selected_at "$workflow_selection_selected_at"
}

assert_script_sha256() {
	label="$1"
	path_key="$2"
	digest_key="$3"
	expected_path="$4"
	path="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" "$path_key")"
	expected="$(assert_sha256 "$digest_key" "$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" "$digest_key")")"
	[ "$path" = "$expected_path" ] || fail "$path_key must reference expected script $expected_path: $path"
	assert_absolute_path "$label" "$path"
	assert_not_symlink_file "$label" "$path"
	assert_no_symlink_path_components "$label" "$path"
	[ -f "$path" ] || fail "$label file is missing: $path"
	actual="$(file_sha256 "$path")"
	[ "$actual" = "$expected" ] || fail "$digest_key mismatch expected $expected got $actual"
}

assert_stage_workspace_dir() {
	label="$1"
	path="$2"
	assert_absolute_path "$label" "$path"
	assert_not_symlink_dir "$label" "$path"
	assert_no_symlink_path_components "$label" "$path"
	[ -d "$path" ] || fail "$label directory is missing: $path"
}

assert_stage_workspace_file() {
	label="$1"
	path="$2"
	assert_not_symlink_file "$label" "$path"
	assert_no_symlink_path_components "$label" "$path"
	[ -f "$path" ] || fail "$label is missing: $path"
}

assert_stage_workspace_path_exists() {
	label="$1"
	path="$2"
	[ ! -L "$path" ] || fail "$label path must not be a symlink: $path"
	assert_no_symlink_path_components "$label" "$path"
	[ -e "$path" ] || fail "$label is missing: $path"
}

assert_no_symlink_parent_path_components() {
	label="$1"
	path="$2"
	parent="$(dirname -- "$path")"
	assert_no_symlink_path_components "$label" "$parent"
}

absolute_directory_path() {
	label="$1"
	path="$2"
	[ -d "$path" ] || fail "$label directory is missing: $path"
	(CDPATH= cd -- "$path" && pwd) || fail "resolve $label directory: $path"
}

assert_stage_workspace_expected_child() {
	workspace_child_field="$1"
	workspace_child_path="$2"
	workspace_child_root="$3"
	workspace_child_name="$4"
	workspace_child_abs="$(absolute_directory_path "$workspace_child_field" "$workspace_child_path")"
	case "$workspace_child_root" in
		/) workspace_child_expected="/$workspace_child_name" ;;
		*) workspace_child_expected="$workspace_child_root/$workspace_child_name" ;;
	esac
	[ "$workspace_child_abs" = "$workspace_child_expected" ] || fail "$workspace_child_field must be under production workspace at $workspace_child_expected: $workspace_child_path"
}

assert_stage_workspace_symlink_target() {
	symlink_label="$1"
	symlink_path="$2"
	symlink_expected_target="$3"
	assert_absolute_path "$symlink_label" "$symlink_path"
	assert_no_symlink_parent_path_components "$symlink_label" "$symlink_path"
	assert_absolute_path "$symlink_label expected target" "$symlink_expected_target"
	assert_no_symlink_path_components "$symlink_label expected target" "$symlink_expected_target"
	[ -L "$symlink_path" ] || fail "$symlink_label must be a symlink: $symlink_path"
	target="$(readlink "$symlink_path")" || fail "read $symlink_label symlink: $symlink_path"
	case "$target" in
		/*) target_path="$target" ;;
		*) target_path="$(dirname -- "$symlink_path")/$target" ;;
	esac
	assert_no_symlink_path_components "$symlink_label target" "$target_path"
	target_abs="$(absolute_directory_path "$symlink_label target" "$target_path")"
	expected_abs="$(absolute_directory_path "$symlink_label expected target" "$symlink_expected_target")"
	[ "$target_abs" = "$expected_abs" ] || fail "$symlink_label target mismatch expected $expected_abs got $target_abs"
}

completion_audit_trusted_shell_path() {
	for candidate in /bin/sh /usr/bin/sh; do
		if [ -x "$candidate" ] && [ ! -d "$candidate" ]; then
			printf '%s\n' "$candidate"
			return 0
		fi
	done
	fail "completion audit trusted shell is missing"
}

[ -n "${OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY:-}" ] || fail "OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY is required"
[ -n "${OURO_COMPLETION_AUDIT_ATTESTATION:-}" ] || fail "OURO_COMPLETION_AUDIT_ATTESTATION is required"
assert_absolute_path "production summary" "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY"
assert_absolute_path "completion attestation" "$OURO_COMPLETION_AUDIT_ATTESTATION"
assert_not_symlink_file "production summary" "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY"
assert_not_symlink_file "completion attestation" "$OURO_COMPLETION_AUDIT_ATTESTATION"
assert_no_symlink_path_components "production summary" "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY"
assert_no_symlink_path_components "completion attestation" "$OURO_COMPLETION_AUDIT_ATTESTATION"
[ -f "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" ] || fail "production summary is missing: $OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY"
[ -f "$OURO_COMPLETION_AUDIT_ATTESTATION" ] || fail "completion attestation is missing: $OURO_COMPLETION_AUDIT_ATTESTATION"
validate_key_value_file "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" "production summary"
validate_key_value_file "$OURO_COMPLETION_AUDIT_ATTESTATION" "completion attestation"

assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" schema ouroboros.production_acceptance.audit.v1
SUMMARY_STARTED_AT="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" started_at)"
assert_rfc3339_timestamp started_at "$SUMMARY_STARTED_AT"
SUMMARY_COMPLETED_AT="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" completed_at)"
assert_rfc3339_timestamp completed_at "$SUMMARY_COMPLETED_AT"
assert_timestamp_not_before started_at "$SUMMARY_STARTED_AT" completed_at "$SUMMARY_COMPLETED_AT"
assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" status accepted
assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" mode operator-owned
assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" ide_smoke_acceptance passed
assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" release_managed_acceptance passed
assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" managed_payload_acceptance passed
assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" workflow_closure_acceptance passed
assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" no_live_targets true
assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" payload_execution_allowed false
assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" network_download_allowed false
assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" package_manager_allowed false
assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" provider_calls_allowed false
assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" worker_contact_allowed false
assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" trusted_path "$TRUSTED_PATH"
TRUSTED_SHELL_FIELD="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" trusted_shell)"
case "$TRUSTED_SHELL_FIELD" in
	/bin/sh|/usr/bin/sh) ;;
	*) fail "trusted_shell must be /bin/sh or /usr/bin/sh: $TRUSTED_SHELL_FIELD" ;;
esac
[ -x "$TRUSTED_SHELL_FIELD" ] || fail "trusted_shell is not executable: $TRUSTED_SHELL_FIELD"
assert_script_sha256 production_acceptance_script production_acceptance_script production_acceptance_script_sha256 "$ROOT/scripts/smoke_production_acceptance.sh"
assert_script_sha256 ide_smoke_script ide_smoke_script ide_smoke_script_sha256 "$ROOT/scripts/smoke_ide.sh"
assert_script_sha256 ide_direct_use_script ide_direct_use_script ide_direct_use_script_sha256 "$ROOT/scripts/smoke_direct_use.sh"
assert_script_sha256 ide_ai_provider_script ide_ai_provider_script ide_ai_provider_script_sha256 "$ROOT/scripts/smoke_ai_provider.sh"
assert_script_sha256 release_managed_script release_managed_script release_managed_script_sha256 "$ROOT/scripts/smoke_release_managed.sh"
assert_script_sha256 managed_payloads_script managed_payloads_script managed_payloads_script_sha256 "$ROOT/scripts/smoke_managed_payloads.sh"
assert_script_sha256 workflow_closure_script workflow_closure_script workflow_closure_script_sha256 "$ROOT/scripts/smoke_workflow_closure.sh"

IDE_LOG_PATH="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" ide_smoke_log)"
IDE_DIRECT_USE_LOG_PATH="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" ide_direct_use_log)"
IDE_AI_PROVIDER_LOG_PATH="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" ide_ai_provider_log)"
RELEASE_LOG_PATH="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" release_managed_log)"
MANAGED_LOG_PATH="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" managed_payloads_log)"
WORKFLOW_LOG_PATH="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" workflow_closure_log)"
IDE_LOG_SHA256="$(assert_sha256 ide_smoke_log_sha256 "$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" ide_smoke_log_sha256)")"
IDE_DIRECT_USE_LOG_SHA256="$(assert_sha256 ide_direct_use_log_sha256 "$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" ide_direct_use_log_sha256)")"
IDE_AI_PROVIDER_LOG_SHA256="$(assert_sha256 ide_ai_provider_log_sha256 "$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" ide_ai_provider_log_sha256)")"
RELEASE_LOG_SHA256="$(assert_sha256 release_managed_log_sha256 "$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" release_managed_log_sha256)")"
MANAGED_LOG_SHA256="$(assert_sha256 managed_payloads_log_sha256 "$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" managed_payloads_log_sha256)")"
WORKFLOW_LOG_SHA256="$(assert_sha256 workflow_closure_log_sha256 "$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" workflow_closure_log_sha256)")"
assert_file_sha256 ide_smoke_log_sha256 "$IDE_LOG_PATH" "$IDE_LOG_SHA256"
assert_file_sha256 ide_direct_use_log_sha256 "$IDE_DIRECT_USE_LOG_PATH" "$IDE_DIRECT_USE_LOG_SHA256"
assert_file_sha256 ide_ai_provider_log_sha256 "$IDE_AI_PROVIDER_LOG_PATH" "$IDE_AI_PROVIDER_LOG_SHA256"
assert_file_sha256 release_managed_log_sha256 "$RELEASE_LOG_PATH" "$RELEASE_LOG_SHA256"
assert_file_sha256 managed_payloads_log_sha256 "$MANAGED_LOG_PATH" "$MANAGED_LOG_SHA256"
assert_file_sha256 workflow_closure_log_sha256 "$WORKFLOW_LOG_PATH" "$WORKFLOW_LOG_SHA256"
assert_file_contains_marker ide_smoke_log "$IDE_LOG_PATH" "ide-smoke: ok"
assert_file_contains_marker ide_direct_use_log "$IDE_DIRECT_USE_LOG_PATH" "direct-use-smoke: ok"
assert_file_contains_marker ide_direct_use_log "$IDE_DIRECT_USE_LOG_PATH" "chat: available"
assert_file_contains_marker ide_direct_use_log "$IDE_DIRECT_USE_LOG_PATH" "chat: composer"
assert_file_contains_marker ide_ai_provider_log "$IDE_AI_PROVIDER_LOG_PATH" "ai-provider-smoke: ok"
assert_file_contains_marker ide_ai_provider_log "$IDE_AI_PROVIDER_LOG_PATH" "chat-prompt: preview=created stored=false"
assert_file_contains_marker release_managed_log "$RELEASE_LOG_PATH" "release-managed-smoke: ok"
assert_file_contains_marker managed_payloads_log "$MANAGED_LOG_PATH" "managed-payloads-smoke: ok"
assert_file_contains_marker workflow_closure_log "$WORKFLOW_LOG_PATH" "workflow-closure-smoke: ok"
assert_summary_stage_log_path ide_smoke_log "$IDE_LOG_PATH" ide-smoke.txt
assert_summary_stage_log_path ide_direct_use_log "$IDE_DIRECT_USE_LOG_PATH" ide-smoke-logs/direct-use.txt
assert_summary_stage_log_path ide_ai_provider_log "$IDE_AI_PROVIDER_LOG_PATH" ide-smoke-logs/ai-provider.txt
assert_summary_stage_log_path release_managed_log "$RELEASE_LOG_PATH" release-managed.txt
assert_summary_stage_log_path managed_payloads_log "$MANAGED_LOG_PATH" managed-payloads.txt
assert_summary_stage_log_path workflow_closure_log "$WORKFLOW_LOG_PATH" workflow-closure.txt

INVENTORY_PATH="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" inventory)"
POLICY_PATH="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" policy)"
INVENTORY_SHA256="$(assert_sha256 inventory_sha256 "$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" inventory_sha256)")"
POLICY_SHA256="$(assert_sha256 policy_sha256 "$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" policy_sha256)")"
assert_absolute_path "referenced inventory" "$INVENTORY_PATH"
assert_absolute_path "referenced policy" "$POLICY_PATH"
assert_not_symlink_file "referenced inventory" "$INVENTORY_PATH"
assert_not_symlink_file "referenced policy" "$POLICY_PATH"
assert_no_symlink_path_components "referenced inventory" "$INVENTORY_PATH"
assert_no_symlink_path_components "referenced policy" "$POLICY_PATH"
[ -f "$INVENTORY_PATH" ] || fail "referenced inventory is missing: $INVENTORY_PATH"
[ -f "$POLICY_PATH" ] || fail "referenced policy is missing: $POLICY_PATH"
validate_key_value_file "$INVENTORY_PATH" "production inventory"
validate_key_value_file "$POLICY_PATH" "production policy"
assert_field_equals "$INVENTORY_PATH" schema ouroboros.production_acceptance.inventory.v1
assert_field_equals "$INVENTORY_PATH" inventory_status operator_owned
BINARY_NAME="$(required_field "$INVENTORY_PATH" binary_name)"
required_field "$INVENTORY_PATH" binary_pack >/dev/null
BINARY_PAYLOAD="$(required_field "$INVENTORY_PATH" binary_payload)"
assert_absolute_path "referenced binary payload" "$BINARY_PAYLOAD"
assert_not_symlink_file "referenced binary payload" "$BINARY_PAYLOAD"
assert_no_symlink_path_components "referenced binary payload" "$BINARY_PAYLOAD"
[ -f "$BINARY_PAYLOAD" ] || fail "referenced binary payload is not a regular file: $BINARY_PAYLOAD"
BINARY_EXPECTED_SHA256="$(assert_sha256 binary_expected_sha256 "$(required_field "$INVENTORY_PATH" binary_expected_sha256)")"
assert_file_sha256 "referenced binary payload" "$BINARY_PAYLOAD" "$BINARY_EXPECTED_SHA256"
REFERENCED_BINARY_SHA256="$BINARY_EXPECTED_SHA256"
required_field "$INVENTORY_PATH" binary_version >/dev/null
required_field "$INVENTORY_PATH" binary_source >/dev/null
RULE_NAME="$(required_field "$INVENTORY_PATH" rule_bundle_name)"
required_field "$INVENTORY_PATH" rule_bundle_pack >/dev/null
RULE_PAYLOAD="$(required_field "$INVENTORY_PATH" rule_bundle_payload)"
assert_absolute_path "referenced rule bundle payload" "$RULE_PAYLOAD"
assert_not_symlink_file "referenced rule bundle payload" "$RULE_PAYLOAD"
assert_no_symlink_path_components "referenced rule bundle payload" "$RULE_PAYLOAD"
[ -d "$RULE_PAYLOAD" ] || fail "referenced rule bundle payload is not a directory: $RULE_PAYLOAD"
RULE_EXPECTED_SHA256="$(assert_sha256 rule_bundle_expected_sha256 "$(required_field "$INVENTORY_PATH" rule_bundle_expected_sha256)")"
REFERENCED_RULE_SHA256="$(directory_sha256 "referenced rule bundle payload" "$RULE_PAYLOAD")"
[ "$REFERENCED_RULE_SHA256" = "$RULE_EXPECTED_SHA256" ] || fail "referenced rule bundle payload mismatch expected $RULE_EXPECTED_SHA256 got $REFERENCED_RULE_SHA256"
required_field "$INVENTORY_PATH" rule_bundle_version >/dev/null
required_field "$INVENTORY_PATH" rule_bundle_source >/dev/null
assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" binary_name "$BINARY_NAME"
SUMMARY_BINARY_EXPECTED_SHA256="$(assert_sha256 binary_expected_sha256 "$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" binary_expected_sha256)")"
[ "$SUMMARY_BINARY_EXPECTED_SHA256" = "$BINARY_EXPECTED_SHA256" ] || fail "binary_expected_sha256 expected $BINARY_EXPECTED_SHA256 got $SUMMARY_BINARY_EXPECTED_SHA256"
assert_field_equals "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" rule_bundle_name "$RULE_NAME"
SUMMARY_RULE_EXPECTED_SHA256="$(assert_sha256 rule_bundle_expected_sha256 "$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" rule_bundle_expected_sha256)")"
[ "$SUMMARY_RULE_EXPECTED_SHA256" = "$RULE_EXPECTED_SHA256" ] || fail "rule_bundle_expected_sha256 expected $RULE_EXPECTED_SHA256 got $SUMMARY_RULE_EXPECTED_SHA256"
assert_field_equals "$POLICY_PATH" schema ouroboros.production_acceptance.policy.v1
assert_field_equals "$POLICY_PATH" policy_status approved
assert_field_equals "$POLICY_PATH" managed_payload_acceptance_required true
assert_field_equals "$POLICY_PATH" workflow_closure_required true
assert_field_equals "$POLICY_PATH" no_live_targets true
assert_field_equals "$POLICY_PATH" payload_execution_allowed false
assert_field_equals "$POLICY_PATH" network_download_allowed false
assert_field_equals "$POLICY_PATH" package_manager_allowed false
assert_field_equals "$POLICY_PATH" provider_calls_allowed false
assert_field_equals "$POLICY_PATH" worker_contact_allowed false
required_field "$POLICY_PATH" deployment_environment >/dev/null
required_field "$POLICY_PATH" accepted_by >/dev/null
POLICY_ACCEPTED_AT="$(required_field "$POLICY_PATH" accepted_at)"
assert_rfc3339_timestamp accepted_at "$POLICY_ACCEPTED_AT"
assert_timestamp_not_after accepted_at "$POLICY_ACCEPTED_AT" started_at "$SUMMARY_STARTED_AT"
required_field "$POLICY_PATH" authorization_scope >/dev/null
assert_file_sha256 inventory_sha256 "$INVENTORY_PATH" "$INVENTORY_SHA256"
assert_file_sha256 policy_sha256 "$POLICY_PATH" "$POLICY_SHA256"

WORKSPACE_PATH="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" workspace)"
IDE_WORKSPACE="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" ide_smoke_workspace)"
RELEASE_WORKSPACE="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" release_managed_workspace)"
MANAGED_WORKSPACE="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" managed_payloads_workspace)"
WORKFLOW_WORKSPACE="$(required_field "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY" workflow_closure_workspace)"
assert_stage_workspace_dir production_workspace "$WORKSPACE_PATH"
assert_stage_workspace_dir ide_smoke_workspace "$IDE_WORKSPACE"
assert_stage_workspace_dir release_managed_workspace "$RELEASE_WORKSPACE"
assert_stage_workspace_dir managed_payloads_workspace "$MANAGED_WORKSPACE"
assert_stage_workspace_dir workflow_closure_workspace "$WORKFLOW_WORKSPACE"
WORKSPACE_ABS="$(absolute_directory_path production_workspace "$WORKSPACE_PATH")"
assert_stage_workspace_expected_child ide_smoke_workspace "$IDE_WORKSPACE" "$WORKSPACE_ABS" ide-smoke-workspace
assert_stage_workspace_expected_child release_managed_workspace "$RELEASE_WORKSPACE" "$WORKSPACE_ABS" release-managed-workspace
assert_stage_workspace_expected_child managed_payloads_workspace "$MANAGED_WORKSPACE" "$WORKSPACE_ABS" managed-payloads-workspace
assert_stage_workspace_expected_child workflow_closure_workspace "$WORKFLOW_WORKSPACE" "$WORKSPACE_ABS" workflow-closure-workspace
assert_stage_workspace_symlink_target "release managed workspace rule link" "$RELEASE_WORKSPACE/toolchain/rules/$RULE_NAME" "$RELEASE_WORKSPACE/toolchain/managed-bundles/$RULE_NAME/payload"
RELEASE_RULE_PAYLOAD="$RELEASE_WORKSPACE/toolchain/managed-bundles/$RULE_NAME/payload"
RELEASE_RULE_SHA256="$(directory_sha256 "release managed workspace rule payload" "$RELEASE_RULE_PAYLOAD")"
[ "$RELEASE_RULE_SHA256" = "$RULE_EXPECTED_SHA256" ] || fail "release managed workspace rule payload mismatch expected $RULE_EXPECTED_SHA256 got $RELEASE_RULE_SHA256"
assert_file_sha256 "managed payload workspace binary payload" "$MANAGED_WORKSPACE/toolchain/managed-bundles/$BINARY_NAME/payload/$BINARY_NAME" "$BINARY_EXPECTED_SHA256"
WORKFLOW_SELECTION_PATH="$WORKFLOW_WORKSPACE/reports/report-projection-selection.json"
assert_stage_workspace_file "workflow closure workspace report selection" "$WORKFLOW_SELECTION_PATH"
assert_json_status_values_current "workflow closure workspace report selection" "$WORKFLOW_SELECTION_PATH"
assert_workflow_report_selection_provenance "workflow closure workspace report selection" "$WORKFLOW_SELECTION_PATH" "$WORKFLOW_WORKSPACE" "$SUMMARY_STARTED_AT" "$SUMMARY_COMPLETED_AT"
WORKFLOW_SELECTION_SHA256="$(file_sha256 "$WORKFLOW_SELECTION_PATH")"

assert_field_equals "$OURO_COMPLETION_AUDIT_ATTESTATION" schema ouroboros.completion_audit.attestation.v1
assert_field_equals "$OURO_COMPLETION_AUDIT_ATTESTATION" attestation_status approved
assert_field_equals "$OURO_COMPLETION_AUDIT_ATTESTATION" production_evidence operator_owned
assert_field_equals "$OURO_COMPLETION_AUDIT_ATTESTATION" fixture_evidence_allowed false
assert_field_equals "$OURO_COMPLETION_AUDIT_ATTESTATION" scope_reviewed true
assert_field_equals "$OURO_COMPLETION_AUDIT_ATTESTATION" safety_boundaries_reviewed true
assert_field_equals "$OURO_COMPLETION_AUDIT_ATTESTATION" release_managed_reviewed true
assert_field_equals "$OURO_COMPLETION_AUDIT_ATTESTATION" managed_payloads_reviewed true
assert_field_equals "$OURO_COMPLETION_AUDIT_ATTESTATION" workflow_closure_reviewed true
assert_field_equals "$OURO_COMPLETION_AUDIT_ATTESTATION" documentation_reviewed true

REVIEWED_BY="$(required_field "$OURO_COMPLETION_AUDIT_ATTESTATION" reviewed_by)"
REVIEWED_AT="$(required_field "$OURO_COMPLETION_AUDIT_ATTESTATION" reviewed_at)"
assert_rfc3339_timestamp reviewed_at "$REVIEWED_AT"
assert_timestamp_not_before completed_at "$SUMMARY_COMPLETED_AT" reviewed_at "$REVIEWED_AT"
assert_timestamp_not_after reviewed_at "$REVIEWED_AT" audit_started_at "$STARTED_AT"
SUMMARY_SHA256="$(file_sha256 "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY")"
assert_field_equals "$OURO_COMPLETION_AUDIT_ATTESTATION" production_summary_sha256 "$SUMMARY_SHA256"
ATTESTATION_SHA256="$(file_sha256 "$OURO_COMPLETION_AUDIT_ATTESTATION")"
COMPLETION_AUDIT_TRUSTED_SHELL="$(completion_audit_trusted_shell_path)"
COMPLETION_AUDIT_TRUSTED_PATH="$TRUSTED_PATH"
assert_absolute_path "completion audit script" "$COMPLETION_AUDIT_SCRIPT"
assert_not_symlink_file "completion audit script" "$COMPLETION_AUDIT_SCRIPT"
assert_no_symlink_path_components "completion audit script" "$COMPLETION_AUDIT_SCRIPT"
[ -f "$COMPLETION_AUDIT_SCRIPT" ] || fail "completion audit script file is missing: $COMPLETION_AUDIT_SCRIPT"
COMPLETION_AUDIT_SCRIPT_SHA256="$(file_sha256 "$COMPLETION_AUDIT_SCRIPT")"
COMPLETED_AT="$(timestamp_utc)"

assert_output_file_path "completion audit result" "$RESULT_PATH"
RESULT_TMP_PATH="$(mktemp "$OURO_COMPLETION_AUDIT_LOG_DIR/.completion-audit.txt.XXXXXX")" || fail "unable to create completion audit result temporary file"
{
	printf 'schema=ouroboros.completion_audit.result.v1\n'
	printf 'audit_id=%s\n' "$AUDIT_ID"
	printf 'started_at=%s\n' "$STARTED_AT"
	printf 'completed_at=%s\n' "$COMPLETED_AT"
	printf 'status=ready-for-final-review\n'
	printf 'completion_audit_trusted_shell=%s\n' "$COMPLETION_AUDIT_TRUSTED_SHELL"
	printf 'completion_audit_trusted_path=%s\n' "$COMPLETION_AUDIT_TRUSTED_PATH"
	printf 'completion_audit_script=%s\n' "$COMPLETION_AUDIT_SCRIPT"
	printf 'completion_audit_script_sha256=%s\n' "$COMPLETION_AUDIT_SCRIPT_SHA256"
	printf 'production_summary=%s\n' "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY"
	printf 'production_summary_sha256=%s\n' "$SUMMARY_SHA256"
	printf 'attestation=%s\n' "$OURO_COMPLETION_AUDIT_ATTESTATION"
	printf 'attestation_sha256=%s\n' "$ATTESTATION_SHA256"
	printf 'evidence_manifest=%s\n' "$MANIFEST_PATH"
	printf 'evidence_manifest_digest=%s\n' "$MANIFEST_DIGEST_PATH"
	printf 'reviewed_by=%s\n' "$REVIEWED_BY"
	printf 'reviewed_at=%s\n' "$REVIEWED_AT"
	printf 'production_acceptance_status=accepted\n'
	printf 'operator_attestation_status=approved\n'
	printf 'ide_smoke_acceptance=passed\n'
	printf 'release_managed_acceptance=passed\n'
	printf 'managed_payload_acceptance=passed\n'
	printf 'workflow_closure_acceptance=passed\n'
	printf 'ide_direct_use_script=%s\n' "$ROOT/scripts/smoke_direct_use.sh"
	printf 'ide_direct_use_script_sha256=%s\n' "$(file_sha256 "$ROOT/scripts/smoke_direct_use.sh")"
	printf 'ide_ai_provider_script=%s\n' "$ROOT/scripts/smoke_ai_provider.sh"
	printf 'ide_ai_provider_script_sha256=%s\n' "$(file_sha256 "$ROOT/scripts/smoke_ai_provider.sh")"
	printf 'ide_smoke_log=%s\n' "$IDE_LOG_PATH"
	printf 'ide_direct_use_log=%s\n' "$IDE_DIRECT_USE_LOG_PATH"
	printf 'ide_ai_provider_log=%s\n' "$IDE_AI_PROVIDER_LOG_PATH"
	printf 'release_managed_log=%s\n' "$RELEASE_LOG_PATH"
	printf 'managed_payloads_log=%s\n' "$MANAGED_LOG_PATH"
	printf 'workflow_closure_log=%s\n' "$WORKFLOW_LOG_PATH"
	printf 'inventory_sha256=%s\n' "$INVENTORY_SHA256"
	printf 'policy_sha256=%s\n' "$POLICY_SHA256"
	printf 'ide_smoke_log_sha256=%s\n' "$IDE_LOG_SHA256"
	printf 'ide_direct_use_log_sha256=%s\n' "$IDE_DIRECT_USE_LOG_SHA256"
	printf 'ide_ai_provider_log_sha256=%s\n' "$IDE_AI_PROVIDER_LOG_SHA256"
	printf 'release_managed_log_sha256=%s\n' "$RELEASE_LOG_SHA256"
	printf 'managed_payloads_log_sha256=%s\n' "$MANAGED_LOG_SHA256"
	printf 'workflow_closure_log_sha256=%s\n' "$WORKFLOW_LOG_SHA256"
	printf 'remaining_manual_step=claim_100_after_real_operator_evidence_review\n'
} > "$RESULT_TMP_PATH"
mv -f "$RESULT_TMP_PATH" "$RESULT_PATH" || fail "unable to publish completion audit result"
RESULT_TMP_PATH=""

RESULT_SHA256="$(file_sha256 "$RESULT_PATH")"
assert_output_file_path "completion audit manifest" "$MANIFEST_PATH"
MANIFEST_TMP_PATH="$(mktemp "$OURO_COMPLETION_AUDIT_LOG_DIR/.completion-evidence-manifest.txt.XXXXXX")" || fail "unable to create completion audit manifest temporary file"
{
	printf 'schema=ouroboros.completion_audit.evidence_manifest.v1\n'
	printf 'audit_id=%s\n' "$AUDIT_ID"
	printf 'completion_audit_started_at=%s\n' "$STARTED_AT"
	printf 'completion_audit_completed_at=%s\n' "$COMPLETED_AT"
	printf 'completed_at=%s\n' "$COMPLETED_AT"
	printf 'status=ready-for-final-review\n'
	printf 'result=%s\n' "$RESULT_PATH"
	printf 'result_sha256=%s\n' "$RESULT_SHA256"
	printf 'completion_audit_trusted_shell=%s\n' "$COMPLETION_AUDIT_TRUSTED_SHELL"
	printf 'completion_audit_trusted_path=%s\n' "$COMPLETION_AUDIT_TRUSTED_PATH"
	printf 'completion_audit_script=%s\n' "$COMPLETION_AUDIT_SCRIPT"
	printf 'completion_audit_script_sha256=%s\n' "$COMPLETION_AUDIT_SCRIPT_SHA256"
	printf 'production_summary=%s\n' "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY"
	printf 'production_summary_sha256=%s\n' "$SUMMARY_SHA256"
	printf 'production_summary_started_at=%s\n' "$SUMMARY_STARTED_AT"
	printf 'production_summary_completed_at=%s\n' "$SUMMARY_COMPLETED_AT"
	printf 'attestation=%s\n' "$OURO_COMPLETION_AUDIT_ATTESTATION"
	printf 'attestation_sha256=%s\n' "$ATTESTATION_SHA256"
	printf 'attestation_reviewed_by=%s\n' "$REVIEWED_BY"
	printf 'attestation_reviewed_at=%s\n' "$REVIEWED_AT"
	printf 'inventory=%s\n' "$INVENTORY_PATH"
	printf 'inventory_sha256=%s\n' "$INVENTORY_SHA256"
	printf 'policy=%s\n' "$POLICY_PATH"
	printf 'policy_sha256=%s\n' "$POLICY_SHA256"
	printf 'policy_accepted_at=%s\n' "$POLICY_ACCEPTED_AT"
	printf 'referenced_binary_payload=%s\n' "$BINARY_PAYLOAD"
	printf 'referenced_binary_payload_sha256=%s\n' "$REFERENCED_BINARY_SHA256"
	printf 'referenced_rule_bundle_payload=%s\n' "$RULE_PAYLOAD"
	printf 'referenced_rule_bundle_payload_sha256=%s\n' "$REFERENCED_RULE_SHA256"
	printf 'referenced_rule_bundle_payload_digest_scope=directory_canonical_records\n'
	printf 'production_workspace=%s\n' "$WORKSPACE_PATH"
	printf 'stage_workspace_relationship=fixed_production_children\n'
	printf 'ide_smoke_log=%s\n' "$IDE_LOG_PATH"
	printf 'ide_smoke_log_sha256=%s\n' "$IDE_LOG_SHA256"
	printf 'ide_direct_use_script=%s\n' "$ROOT/scripts/smoke_direct_use.sh"
	printf 'ide_direct_use_script_sha256=%s\n' "$(file_sha256 "$ROOT/scripts/smoke_direct_use.sh")"
	printf 'ide_direct_use_log=%s\n' "$IDE_DIRECT_USE_LOG_PATH"
	printf 'ide_direct_use_log_sha256=%s\n' "$IDE_DIRECT_USE_LOG_SHA256"
	printf 'ide_ai_provider_script=%s\n' "$ROOT/scripts/smoke_ai_provider.sh"
	printf 'ide_ai_provider_script_sha256=%s\n' "$(file_sha256 "$ROOT/scripts/smoke_ai_provider.sh")"
	printf 'ide_ai_provider_log=%s\n' "$IDE_AI_PROVIDER_LOG_PATH"
	printf 'ide_ai_provider_log_sha256=%s\n' "$IDE_AI_PROVIDER_LOG_SHA256"
	printf 'release_managed_log=%s\n' "$RELEASE_LOG_PATH"
	printf 'release_managed_log_sha256=%s\n' "$RELEASE_LOG_SHA256"
	printf 'managed_payloads_log=%s\n' "$MANAGED_LOG_PATH"
	printf 'managed_payloads_log_sha256=%s\n' "$MANAGED_LOG_SHA256"
	printf 'workflow_closure_log=%s\n' "$WORKFLOW_LOG_PATH"
		printf 'workflow_closure_log_sha256=%s\n' "$WORKFLOW_LOG_SHA256"
		printf 'ide_smoke_workspace=%s\n' "$IDE_WORKSPACE"
		printf 'release_managed_workspace=%s\n' "$RELEASE_WORKSPACE"
		printf 'release_managed_rule_link=%s\n' "$RELEASE_WORKSPACE/toolchain/rules/$RULE_NAME"
		printf 'release_managed_rule_target=%s\n' "$RELEASE_RULE_PAYLOAD"
		printf 'release_managed_rule_payload_sha256=%s\n' "$RELEASE_RULE_SHA256"
		printf 'release_managed_rule_payload_digest_scope=directory_canonical_records\n'
		printf 'managed_payloads_workspace=%s\n' "$MANAGED_WORKSPACE"
		printf 'managed_payload_binary=%s\n' "$MANAGED_WORKSPACE/toolchain/managed-bundles/$BINARY_NAME/payload/$BINARY_NAME"
		printf 'managed_payload_binary_sha256=%s\n' "$BINARY_EXPECTED_SHA256"
		printf 'workflow_closure_workspace=%s\n' "$WORKFLOW_WORKSPACE"
		printf 'workflow_report_selection=%s\n' "$WORKFLOW_SELECTION_PATH"
		printf 'workflow_report_selection_sha256=%s\n' "$WORKFLOW_SELECTION_SHA256"
		printf 'workflow_report_execution=%s\n' "$workflow_selection_execution_path"
		printf 'workflow_report_execution_sha256=%s\n' "$workflow_selection_execution_sha256"
		printf 'workflow_report_dispatch=%s\n' "$workflow_selection_dispatch_path"
		printf 'workflow_report_dispatch_sha256=%s\n' "$workflow_selection_dispatch_sha256"
		printf 'workflow_report_run_id=%s\n' "$workflow_selection_run_id"
		printf 'workflow_report_path=%s\n' "$workflow_selection_report_path"
		printf 'workflow_report_sha256=%s\n' "$workflow_selection_report_sha256"
		printf 'workflow_report_event_count=%s\n' "$workflow_selection_event_count"
		printf 'workflow_report_dispatch_created_at=%s\n' "$workflow_dispatch_created_at"
		printf 'workflow_report_execution_executed_at=%s\n' "$workflow_execution_executed_at"
		printf 'workflow_report_execution_reviewed_at=%s\n' "$workflow_execution_reviewed_at"
		printf 'workflow_report_selected_at=%s\n' "$workflow_selection_selected_at"
		printf 'remaining_manual_step=claim_100_after_real_operator_evidence_review\n'
	} > "$MANIFEST_TMP_PATH"
mv -f "$MANIFEST_TMP_PATH" "$MANIFEST_PATH" || fail "unable to publish completion audit manifest"
MANIFEST_TMP_PATH=""

MANIFEST_SHA256="$(file_sha256 "$MANIFEST_PATH")"
assert_output_file_path "completion audit manifest digest" "$MANIFEST_DIGEST_PATH"
MANIFEST_DIGEST_TMP_PATH="$(mktemp "$OURO_COMPLETION_AUDIT_LOG_DIR/.completion-evidence-manifest.sha256.XXXXXX")" || fail "unable to create completion audit manifest digest temporary file"
{
	printf 'schema=ouroboros.completion_audit.evidence_manifest_digest.v1\n'
	printf 'audit_id=%s\n' "$AUDIT_ID"
	printf 'completion_audit_started_at=%s\n' "$STARTED_AT"
	printf 'completion_audit_completed_at=%s\n' "$COMPLETED_AT"
	printf 'completed_at=%s\n' "$COMPLETED_AT"
	printf 'status=ready-for-final-review\n'
	printf 'result=%s\n' "$RESULT_PATH"
	printf 'result_sha256=%s\n' "$RESULT_SHA256"
	printf 'evidence_manifest=%s\n' "$MANIFEST_PATH"
	printf 'evidence_manifest_sha256=%s\n' "$MANIFEST_SHA256"
	printf 'attestation_reviewed_by=%s\n' "$REVIEWED_BY"
	printf 'attestation_reviewed_at=%s\n' "$REVIEWED_AT"
	printf 'completion_audit_trusted_shell=%s\n' "$COMPLETION_AUDIT_TRUSTED_SHELL"
	printf 'completion_audit_trusted_path=%s\n' "$COMPLETION_AUDIT_TRUSTED_PATH"
	printf 'completion_audit_script=%s\n' "$COMPLETION_AUDIT_SCRIPT"
	printf 'completion_audit_script_sha256=%s\n' "$COMPLETION_AUDIT_SCRIPT_SHA256"
	printf 'remaining_manual_step=claim_100_after_real_operator_evidence_review\n'
} > "$MANIFEST_DIGEST_TMP_PATH"
mv -f "$MANIFEST_DIGEST_TMP_PATH" "$MANIFEST_DIGEST_PATH" || fail "unable to publish completion audit manifest digest"
MANIFEST_DIGEST_TMP_PATH=""

printf 'completion-audit: production-summary %s\n' "$OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY"
printf 'completion-audit: attestation %s\n' "$OURO_COMPLETION_AUDIT_ATTESTATION"
printf 'completion-audit: production_acceptance=accepted\n'
printf 'completion-audit: operator_attestation=approved\n'
printf 'completion-audit: status=ready-for-final-review\n'
printf 'completion-audit: result %s\n' "$RESULT_PATH"
printf 'completion-audit: evidence-manifest %s\n' "$MANIFEST_PATH"
printf 'completion-audit: evidence-manifest-digest %s\n' "$MANIFEST_DIGEST_PATH"
printf 'completion-audit: ok\n'
