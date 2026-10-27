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

case "$0" in
	*/*) ROOT_DIR="${0%/*}" ;;
	*) ROOT_DIR="." ;;
esac
ROOT="$(CDPATH= cd -- "$ROOT_DIR/.." && pwd)"
GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
TMPDIR_BASE="${TMPDIR:-/tmp}"

if [ -z "${OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR:-}" ]; then
	OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR="$(mktemp -d "$TMPDIR_BASE/ouro-workflow-closure-smoke-logs-XXXXXX")"
fi

fail() {
	printf 'workflow-closure-smoke: failed: %s\n' "$1" >&2
	printf 'workflow-closure-smoke: logs %s\n' "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR" >&2
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

assert_absolute_path "workflow closure log dir" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR"
assert_not_symlink_dir "workflow closure log dir" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR"
mkdir -p "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR"
assert_not_symlink_dir "workflow closure log dir" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR"

workspace_provided=0
if [ -z "${WORKSPACE:-}" ]; then
	SMOKE_ROOT="$(mktemp -d "$TMPDIR_BASE/ouro-workflow-closure-smoke-XXXXXX")"
	WORKSPACE="$SMOKE_ROOT/workspace"
else
	workspace_provided=1
fi
assert_absolute_path "workspace" "$WORKSPACE"
assert_not_symlink_dir "workspace" "$WORKSPACE"

if [ -e "$WORKSPACE" ] && [ ! -d "$WORKSPACE" ]; then
	fail "workspace path exists and is not a directory"
fi

if [ "$workspace_provided" -eq 1 ] && [ -d "$WORKSPACE" ]; then
	existing_entry="$(find "$WORKSPACE" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null || true)"
	if [ -n "$existing_entry" ] && [ "${OURO_WORKFLOW_CLOSURE_SMOKE_ALLOW_EXISTING_WORKSPACE:-}" != "1" ]; then
		fail "refusing existing non-empty workspace; set OURO_WORKFLOW_CLOSURE_SMOKE_ALLOW_EXISTING_WORKSPACE=1 only for a safe smoke workspace"
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

workflow_gate() {
	log_path="$1"
	expected_gate="$2"
	run_ouro_logged "workflow-status $expected_gate" "$log_path" workflow-status "$WORKSPACE"
	status="$(awk '
		/^workflow-status:/ {
			for (i = 1; i <= NF; i++) {
				if ($i ~ /^status=/) {
					print substr($i, 8)
					exit
				}
			}
		}
	' "$log_path")"
	gate="$(awk '
		/^workflow-status:/ {
			for (i = 1; i <= NF; i++) {
				if ($i ~ /^next_gate=/) {
					print substr($i, 11)
					exit
				}
			}
		}
	' "$log_path")"
	[ "$status" = "ready" ] || fail "workflow-status for $expected_gate reported status=$status"
	[ "$gate" = "$expected_gate" ] || fail "workflow-status expected next_gate=$expected_gate got $gate"
	printf 'workflow-status: next_gate=%s\n' "$gate"
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
	[ ! -L "$artifact" ] || fail "workflow-links $key artifact must not be a symlink: $artifact"
	[ -f "$artifact" ] || fail "workflow-links $key artifact was not written: $artifact"
	printf '%s\n' "$artifact"
}

artifact_from_log() {
	log_path="$1"
	artifact="$(awk '/^artifact: / { print substr($0, 11); exit }' "$log_path")"
	[ -n "$artifact" ] || fail "missing artifact line in $log_path"
	[ -f "$artifact" ] || fail "artifact from $log_path was not written: $artifact"
	printf '%s\n' "$artifact"
}

cd "$ROOT"

printf 'workflow-closure-smoke: workspace %s\n' "$WORKSPACE"
printf 'workflow-closure-smoke: logs %s\n' "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR"

run_ouro_logged "init" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/init.txt" init "$WORKSPACE"
run_ouro_logged "strict doctor" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/doctor.txt" doctor "$WORKSPACE" --require-safety-authorization
grep -q '^workspace: ready$' "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/doctor.txt" || fail "strict doctor did not report ready"
printf 'doctor: ready\n'

run_ouro_logged "run --defer-report-projection" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/run.txt" run "$WORKSPACE" --defer-report-projection
RUN_ID="$(awk '/^run: / { print $2; exit }' "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/run.txt")"
REPORT_VALUE="$(awk '/^report: / { print substr($0, 9); exit }' "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/run.txt")"
[ -n "$RUN_ID" ] || fail "run output did not include run id"
[ "$REPORT_VALUE" = "not-generated" ] || fail "deferred run should report not-generated, got $REPORT_VALUE"
printf 'run: %s\n' "$RUN_ID"

workflow_gate "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/workflow-status-create-compile-dispatch.txt" "create-compile-preview-review-execution-dispatch"
REVIEW_ARTIFACT="$(workflow_link_artifact "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/workflow-status-create-compile-dispatch.txt" review)"

run_ouro_logged "compile dispatch" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/compile-dispatch.txt" toolchain compile-preview-review-execution-dispatch "$WORKSPACE" "$REVIEW_ARTIFACT" --actor smoke --reason workflow_closure_smoke
grep -q '^compile-preview-review-execution-dispatch: .* status=dry-run mode=metadata-only' "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/compile-dispatch.txt" || fail "compile dispatch was not dry-run metadata-only"
COMPILE_DISPATCH="$(artifact_from_log "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/compile-dispatch.txt")"
printf 'compile-dispatch: status=dry-run mode=metadata-only\n'

workflow_gate "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/workflow-status-execute-compile-dispatch.txt" "execute-compile-preview-review-execution-dispatch"

run_ouro_logged "compile execution" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/compile-execute.txt" toolchain execute-compile-preview-review-execution-dispatch "$WORKSPACE" "$COMPILE_DISPATCH" --execute --actor smoke --reason workflow_closure_smoke
grep -q '^compile-preview-review-execution: .* status=completed mode=execute' "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/compile-execute.txt" || fail "compile execution did not complete"
COMPILE_EXECUTION="$(artifact_from_log "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/compile-execute.txt")"
REPORT_RUN_ID="$(awk '/^compile-preview-review-execution: / { print $2; exit }' "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/compile-execute.txt")"
[ -n "$REPORT_RUN_ID" ] || fail "compile execution output did not include run id"
[ -f "$COMPILE_EXECUTION" ] || fail "compile execution artifact missing"
printf 'compile-execution: status=completed mode=execute\n'

workflow_gate "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/workflow-status-create-report-dispatch.txt" "create-report-projection-dispatch"

run_ouro_logged "report dispatch" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/report-dispatch.txt" report-project-dispatch "$WORKSPACE" "$REPORT_RUN_ID" --actor smoke --reason workflow_closure_smoke
grep -q '^report-project-dispatch: .* status=dry-run mode=metadata-only' "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/report-dispatch.txt" || fail "report dispatch was not dry-run metadata-only"
REPORT_DISPATCH="$(artifact_from_log "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/report-dispatch.txt")"
printf 'report-dispatch: status=dry-run mode=metadata-only\n'

workflow_gate "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/workflow-status-execute-report-dispatch.txt" "execute-report-projection-dispatch"

run_ouro_logged "report execution" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/report-execute.txt" execute-report-project-dispatch "$WORKSPACE" "$REPORT_DISPATCH" --execute --actor smoke --reason workflow_closure_smoke
grep -q '^report-project-dispatch-execution: .* status=completed mode=execute .* written=true' "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/report-execute.txt" || fail "report execution did not complete and write"
REPORT_EXECUTION="$(artifact_from_log "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/report-execute.txt")"
REPORT_PATH="$(awk '/^report: / { print substr($0, 9); exit }' "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/report-execute.txt")"
[ -n "$REPORT_PATH" ] || fail "report execution output did not include report path"
[ -f "$REPORT_PATH" ] || fail "report execution did not write report: $REPORT_PATH"
printf 'report-execution: status=completed written=true\n'

workflow_gate "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/workflow-status-review-report-execution.txt" "review-report-projection-execution"

run_ouro_logged "report annotation" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/report-annotate.txt" report-project-dispatch-execution-annotate "$WORKSPACE" "$REPORT_EXECUTION" --decision accepted --reviewer smoke --reason workflow_closure_smoke
grep -q '^report-project-dispatch-execution-annotation: decision=accepted' "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/report-annotate.txt" || fail "report execution annotation was not accepted"
printf 'report-annotation: decision=accepted\n'

workflow_gate "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/workflow-status-select-report-execution.txt" "select-report-projection-execution"

run_ouro_logged "report selection" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/report-select.txt" report-project-dispatch-execution-select "$WORKSPACE" "$REPORT_EXECUTION" --selected-by smoke --reason workflow_closure_smoke
grep -q '^report-project-dispatch-execution-selection: status=current' "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/report-select.txt" || fail "report execution was not selected as current"
printf 'report-selection: status=current\n'

workflow_gate "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/workflow-status-review-current-report.txt" "review-current-report-projection"

run_ouro_logged "report selection readback" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/report-selection.txt" report-project-dispatch-execution-selection "$WORKSPACE"
grep -q '^report-project-dispatch-execution-selection: status=current' "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/report-selection.txt" || fail "current report selection readback failed"

run_ouro_logged "closed replay --tui" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/replay.txt" replay "$WORKSPACE" "$REPORT_RUN_ID" --tui
grep -q "^replay: $REPORT_RUN_ID$" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/replay.txt" || fail "closed replay output did not include run id"
assert_tui_snapshot "closed replay --tui" "$OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR/replay.txt"
printf 'replay: ok\n'

printf 'workflow-closure-smoke: ok\n'
