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

if [ -z "${OURO_SMOKE_LOG_DIR:-}" ]; then
	OURO_SMOKE_LOG_DIR="$(mktemp -d "$TMPDIR_BASE/ouro-workflow-smoke-logs-XXXXXX")"
fi

mkdir -p "$OURO_SMOKE_LOG_DIR"

fail() {
	printf 'workflow-smoke: failed: %s\n' "$1" >&2
	printf 'workflow-smoke: logs %s\n' "$OURO_SMOKE_LOG_DIR" >&2
	exit 1
}

workspace_provided=0
if [ -z "${WORKSPACE:-}" ]; then
	SMOKE_ROOT="$(mktemp -d "$TMPDIR_BASE/ouro-workflow-smoke-XXXXXX")"
	WORKSPACE="$SMOKE_ROOT/workspace"
else
	workspace_provided=1
fi

if [ -e "$WORKSPACE" ] && [ ! -d "$WORKSPACE" ]; then
	fail "workspace path exists and is not a directory"
fi

if [ "$workspace_provided" -eq 1 ] && [ -d "$WORKSPACE" ]; then
	existing_entry="$(find "$WORKSPACE" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null || true)"
	if [ -n "$existing_entry" ] && [ "${OURO_SMOKE_ALLOW_EXISTING_WORKSPACE:-}" != "1" ]; then
		fail "refusing existing non-empty workspace; set OURO_SMOKE_ALLOW_EXISTING_WORKSPACE=1 only for a safe smoke workspace"
	fi
fi

# README-(fresh binary) — 防陈旧验收:bin 缺失或源码更新则先 go build;
# OURO_SMOKE_SKIP_BUILD=1 可跳过(默认必须构建)。优先 go build 而非 go run。
OURO_BIN="$ROOT/bin/ouro"
. "$ROOT/scripts/smoke_fresh_bin.sh"
ensure_fresh_ouro_bin "$OURO_BIN" || fail "unable to build fresh ouro binary"
if [ ! -x "$OURO_BIN" ] && [ -x "$OURO_BIN.exe" ]; then
	OURO_BIN="$OURO_BIN.exe"
fi

run_ouro() {
	env GOPROXY="$GOPROXY" "$OURO_BIN" "$@"
}

run_ouro_logged() {
	label="$1"
	log_path="$2"
	shift 2
	if ! run_ouro "$@" > "$log_path" 2>&1; then
		fail "$label failed; see $log_path"
	fi
}

assert_tui_snapshot() {
	label="$1"
	log_path="$2"
	for panel in "Workspace" "Mission Conversation" "Attack Graph" "Runtime Tabs"; do
		grep -q "$panel" "$log_path" || fail "$label output did not include TUI panel: $panel"
	done
}

cd "$ROOT"

printf 'workflow-smoke: workspace %s\n' "$WORKSPACE"
printf 'workflow-smoke: logs %s\n' "$OURO_SMOKE_LOG_DIR"

run_ouro_logged "init" "$OURO_SMOKE_LOG_DIR/init.txt" init "$WORKSPACE"
run_ouro_logged "strict doctor" "$OURO_SMOKE_LOG_DIR/doctor.txt" doctor "$WORKSPACE" --require-safety-authorization
grep -q '^workspace: ready$' "$OURO_SMOKE_LOG_DIR/doctor.txt" || fail "strict doctor did not report ready"
printf 'doctor: ready\n'

run_ouro_logged "run --tui" "$OURO_SMOKE_LOG_DIR/run.txt" run "$WORKSPACE" --tui
RUN_ID="$(awk '/^run: / { print $2; exit }' "$OURO_SMOKE_LOG_DIR/run.txt")"
EVENTS="$(awk '/^events: / { print $2; exit }' "$OURO_SMOKE_LOG_DIR/run.txt")"
REPORT="$(awk '/^report: / { print $2; exit }' "$OURO_SMOKE_LOG_DIR/run.txt")"

[ -n "$RUN_ID" ] || fail "run output did not include run id"
[ -n "$EVENTS" ] || fail "run output did not include event count"
[ -n "$REPORT" ] || fail "run output did not include report path"
[ -f "$REPORT" ] || fail "run report path was not written"
assert_tui_snapshot "run --tui" "$OURO_SMOKE_LOG_DIR/run.txt"

printf 'run: %s\n' "$RUN_ID"
printf 'events: %s\n' "$EVENTS"
printf 'report: %s\n' "$REPORT"

run_ouro_logged "replay --tui" "$OURO_SMOKE_LOG_DIR/replay.txt" replay "$WORKSPACE" "$RUN_ID" --tui
grep -q "^replay: $RUN_ID$" "$OURO_SMOKE_LOG_DIR/replay.txt" || fail "replay output did not include run id"
grep -q '^tools: ready$' "$OURO_SMOKE_LOG_DIR/replay.txt" || fail "replay output did not report ready tools"
assert_tui_snapshot "replay --tui" "$OURO_SMOKE_LOG_DIR/replay.txt"
printf 'replay: ok\n'

run_ouro_logged "workflow-status" "$OURO_SMOKE_LOG_DIR/workflow-status.txt" workflow-status "$WORKSPACE"
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
' "$OURO_SMOKE_LOG_DIR/workflow-status.txt")"
[ -n "$WORKFLOW_STATUS" ] || fail "workflow-status output did not include status and next gate"
printf 'workflow-status: %s\n' "$WORKFLOW_STATUS"

printf 'workflow-smoke: ok\n'
