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
TMPDIR_BASE="${TMPDIR:-/tmp}"
OURO_DIRECT_USE_SMOKE_BIN="${OURO_DIRECT_USE_SMOKE_BIN:-$ROOT/bin/ouro}"

if [ -z "${OURO_DIRECT_USE_SMOKE_LOG_DIR:-}" ]; then
	OURO_DIRECT_USE_SMOKE_LOG_DIR="$(mktemp -d "$TMPDIR_BASE/ouro-direct-use-smoke-logs-XXXXXX")"
fi

fail() {
	printf 'direct-use-smoke: failed: %s\n' "$1" >&2
	printf 'direct-use-smoke: logs %s\n' "$OURO_DIRECT_USE_SMOKE_LOG_DIR" >&2
	exit 1
}

assert_absolute_path() {
	label="$1"
	path="$2"
	case "$path" in
		/*) ;;
		*) fail "$label path must be absolute: $path" ;;
	esac
}

assert_not_symlink_dir() {
	label="$1"
	path="$2"
	[ ! -L "$path" ] || fail "$label directory must not be a symlink: $path"
}

assert_not_symlink_file() {
	label="$1"
	path="$2"
	[ ! -L "$path" ] || fail "$label file must not be a symlink: $path"
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

assert_non_empty_file() {
	label="$1"
	path="$2"
	[ ! -L "$path" ] || fail "$label must not be a symlink: $path"
	[ -s "$path" ] || fail "$label was not written or is empty: $path"
}

assert_tui_snapshot() {
	label="$1"
	log_path="$2"
	for panel in "Workspace" "Mission Conversation" "Attack Graph" "Runtime Tabs"; do
		grep -q "$panel" "$log_path" || fail "$label output did not include TUI panel: $panel"
	done
}

workflow_link_artifact() {
	log_path="$1"
	key="$2"
	artifact="$(awk -v key="$key" '
		/^workflow-links:/ {
			marker = key "="
			start = index($0, marker)
			if (start == 0) {
				exit
			}
			value = substr($0, start + length(marker))
			end = length(value) + 1
			split("preview acceptance review selection dispatch execution report_dispatch report_execution report_selection report_run report", keys, " ")
			for (i in keys) {
				if (keys[i] == key) {
					continue
				}
				pattern = " " keys[i] "="
				pos = index(value, pattern)
				if (pos > 0 && pos < end) {
					end = pos
				}
			}
			print substr(value, 1, end - 1)
			exit
		}
	' "$log_path")"
	[ -n "$artifact" ] || fail "workflow-links missing $key in $log_path"
	assert_non_empty_file "workflow-links $key artifact" "$artifact"
	printf '%s\n' "$artifact"
}

# README-(fresh binary) — 防陈旧验收:bin 缺失或源码更新则先 go build;
# OURO_SMOKE_SKIP_BUILD=1 可跳过(默认必须构建)。
. "$ROOT/scripts/smoke_fresh_bin.sh"
ensure_fresh_ouro_bin "$OURO_DIRECT_USE_SMOKE_BIN" || fail "unable to build fresh ouro binary"

assert_absolute_path "direct use binary" "$OURO_DIRECT_USE_SMOKE_BIN"
assert_no_symlink_path_components "direct use binary" "$OURO_DIRECT_USE_SMOKE_BIN"
assert_not_symlink_file "direct use binary" "$OURO_DIRECT_USE_SMOKE_BIN"
[ -x "$OURO_DIRECT_USE_SMOKE_BIN" ] || fail "direct use binary is not executable: $OURO_DIRECT_USE_SMOKE_BIN"

assert_absolute_path "direct use log dir" "$OURO_DIRECT_USE_SMOKE_LOG_DIR"
assert_no_symlink_path_components "direct use log dir" "$OURO_DIRECT_USE_SMOKE_LOG_DIR"
assert_not_symlink_dir "direct use log dir" "$OURO_DIRECT_USE_SMOKE_LOG_DIR"
mkdir -p "$OURO_DIRECT_USE_SMOKE_LOG_DIR"
assert_no_symlink_path_components "direct use log dir" "$OURO_DIRECT_USE_SMOKE_LOG_DIR"
assert_not_symlink_dir "direct use log dir" "$OURO_DIRECT_USE_SMOKE_LOG_DIR"

workspace_provided=0
if [ -z "${WORKSPACE:-}" ]; then
	SMOKE_ROOT="$(mktemp -d "$TMPDIR_BASE/ouro-direct-use-smoke-XXXXXX")"
	WORKSPACE="$SMOKE_ROOT/workspace"
else
	workspace_provided=1
fi
assert_absolute_path "workspace" "$WORKSPACE"
assert_no_symlink_path_components "workspace" "$WORKSPACE"
assert_not_symlink_dir "workspace" "$WORKSPACE"

if [ -e "$WORKSPACE" ] && [ ! -d "$WORKSPACE" ]; then
	fail "workspace path exists and is not a directory"
fi

if [ "$workspace_provided" -eq 1 ] && [ -d "$WORKSPACE" ]; then
	existing_entry="$(find "$WORKSPACE" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null || true)"
	if [ -n "$existing_entry" ] && [ "${OURO_DIRECT_USE_SMOKE_ALLOW_EXISTING_WORKSPACE:-}" != "1" ]; then
		fail "refusing existing non-empty workspace; set OURO_DIRECT_USE_SMOKE_ALLOW_EXISTING_WORKSPACE=1 only for a safe smoke workspace"
	fi
fi

run_ouro_logged() {
	label="$1"
	log_path="$2"
	shift 2
	if ! "$OURO_DIRECT_USE_SMOKE_BIN" "$@" > "$log_path" 2>&1; then
		fail "$label failed; see $log_path"
	fi
}

cd "$ROOT"

printf 'direct-use-smoke: workspace %s\n' "$WORKSPACE"
printf 'direct-use-smoke: logs %s\n' "$OURO_DIRECT_USE_SMOKE_LOG_DIR"
printf 'binary: %s\n' "$OURO_DIRECT_USE_SMOKE_BIN"

run_ouro_logged "init" "$OURO_DIRECT_USE_SMOKE_LOG_DIR/init.txt" init "$WORKSPACE"

run_ouro_logged "doctor" "$OURO_DIRECT_USE_SMOKE_LOG_DIR/doctor.txt" doctor "$WORKSPACE"
grep -q '^workspace: ready$' "$OURO_DIRECT_USE_SMOKE_LOG_DIR/doctor.txt" || fail "doctor did not report ready"
printf 'doctor: ready\n'

run_ouro_logged "chat --help" "$OURO_DIRECT_USE_SMOKE_LOG_DIR/chat-help.txt" chat --help
grep -q '^Launch the workspace AI chat IDE$' "$OURO_DIRECT_USE_SMOKE_LOG_DIR/chat-help.txt" || fail "chat help did not describe the AI chat IDE entrypoint"
grep -q '^Usage:$' "$OURO_DIRECT_USE_SMOKE_LOG_DIR/chat-help.txt" || fail "chat help did not include usage"
grep -q 'ouro chat <workspace>' "$OURO_DIRECT_USE_SMOKE_LOG_DIR/chat-help.txt" || fail "chat help did not include workspace usage"
printf 'chat: available\n'

run_ouro_logged "chat --tui" "$OURO_DIRECT_USE_SMOKE_LOG_DIR/chat-tui.txt" chat "$WORKSPACE" --tui
assert_tui_snapshot "chat --tui" "$OURO_DIRECT_USE_SMOKE_LOG_DIR/chat-tui.txt"
grep -q 'dialog: ai operator chat' "$OURO_DIRECT_USE_SMOKE_LOG_DIR/chat-tui.txt" || fail "chat --tui output did not include AI dialog status"
grep -q 'AI setup:' "$OURO_DIRECT_USE_SMOKE_LOG_DIR/chat-tui.txt" || fail "chat --tui output did not include AI setup guidance"
grep -q 'You > |' "$OURO_DIRECT_USE_SMOKE_LOG_DIR/chat-tui.txt" || fail "chat --tui output did not include focused input composer"
printf 'chat: composer\n'

run_ouro_logged "run --tui" "$OURO_DIRECT_USE_SMOKE_LOG_DIR/run.txt" run "$WORKSPACE" --tui
RUN_ID="$(awk '/^run: / { print $2; exit }' "$OURO_DIRECT_USE_SMOKE_LOG_DIR/run.txt")"
EVENTS="$(awk '/^events: / { print $2; exit }' "$OURO_DIRECT_USE_SMOKE_LOG_DIR/run.txt")"
REPORT="$(awk '/^report: / { print substr($0, 9); exit }' "$OURO_DIRECT_USE_SMOKE_LOG_DIR/run.txt")"
[ -n "$RUN_ID" ] || fail "run output did not include run id"
[ -n "$EVENTS" ] || fail "run output did not include event count"
[ -n "$REPORT" ] || fail "run output did not include report path"
assert_non_empty_file "run report" "$REPORT"
assert_tui_snapshot "run --tui" "$OURO_DIRECT_USE_SMOKE_LOG_DIR/run.txt"
printf 'run: %s\n' "$RUN_ID"
printf 'events: %s\n' "$EVENTS"
printf 'report: %s\n' "$REPORT"

run_ouro_logged "replay --tui" "$OURO_DIRECT_USE_SMOKE_LOG_DIR/replay.txt" replay "$WORKSPACE" "$RUN_ID" --tui
grep -q "^replay: $RUN_ID$" "$OURO_DIRECT_USE_SMOKE_LOG_DIR/replay.txt" || fail "replay output did not include run id"
grep -q '^tools: ready$' "$OURO_DIRECT_USE_SMOKE_LOG_DIR/replay.txt" || fail "replay output did not report ready tools"
assert_tui_snapshot "replay --tui" "$OURO_DIRECT_USE_SMOKE_LOG_DIR/replay.txt"
printf 'replay: ok\n'

run_ouro_logged "workflow-status" "$OURO_DIRECT_USE_SMOKE_LOG_DIR/workflow-status.txt" workflow-status "$WORKSPACE"
WORKFLOW_STATUS="$(awk '
	/^workflow-status:/ {
		for (i = 1; i <= NF; i++) {
			if ($i ~ /^status=/) {
				status = substr($i, 8)
			}
			if ($i ~ /^next_gate=/) {
				gate = substr($i, 11)
			}
		}
	}
	END {
		if (status != "" || gate != "") {
			printf "status=%s next_gate=%s", status, gate
		}
	}
' "$OURO_DIRECT_USE_SMOKE_LOG_DIR/workflow-status.txt")"
[ -n "$WORKFLOW_STATUS" ] || fail "workflow-status output did not include status and next gate"
printf 'workflow-status: %s\n' "$WORKFLOW_STATUS"

REVIEW_ARTIFACT="$(workflow_link_artifact "$OURO_DIRECT_USE_SMOKE_LOG_DIR/workflow-status.txt" review)"
SELECTION_ARTIFACT="$(workflow_link_artifact "$OURO_DIRECT_USE_SMOKE_LOG_DIR/workflow-status.txt" selection)"
printf 'artifacts: report review selection\n'
printf 'review: %s\n' "$REVIEW_ARTIFACT"
printf 'selection: %s\n' "$SELECTION_ARTIFACT"

printf 'direct-use-smoke: ok\n'
