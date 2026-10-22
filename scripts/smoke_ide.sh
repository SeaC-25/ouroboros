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

TRUSTED_SHELL="/bin/sh"
if [ ! -x "$TRUSTED_SHELL" ]; then
	TRUSTED_SHELL="/usr/bin/sh"
fi
[ -x "$TRUSTED_SHELL" ] || {
	printf 'ide-smoke: failed: trusted shell not found\n' >&2
	exit 1
}

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
TMPDIR_BASE="${TMPDIR:-/tmp}"
OURO_IDE_SMOKE_BIN="${OURO_IDE_SMOKE_BIN:-$ROOT/bin/ouro}"

if [ -z "${OURO_IDE_SMOKE_LOG_DIR:-}" ]; then
	OURO_IDE_SMOKE_LOG_DIR="$(mktemp -d "$TMPDIR_BASE/ouro-ide-smoke-logs-XXXXXX")"
fi

fail() {
	printf 'ide-smoke: failed: %s\n' "$1" >&2
	printf 'ide-smoke: logs %s\n' "$OURO_IDE_SMOKE_LOG_DIR" >&2
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

assert_output_file_path() {
	label="$1"
	path="$2"
	if [ -e "$path" ]; then
		[ ! -L "$path" ] || fail "$label output must not be a symlink: $path"
		[ -f "$path" ] || fail "$label output path exists and is not a regular file: $path"
	fi
}

run_stage_logged() {
	label="$1"
	log_path="$2"
	shift 2
	assert_output_file_path "$label log" "$log_path"
	if ! "$@" > "$log_path" 2>&1; then
		fail "$label failed; see $log_path"
	fi
}

# README-(fresh binary) — 防陈旧验收:bin 缺失或源码更新则先 go build;
# OURO_SMOKE_SKIP_BUILD=1 可跳过(默认必须构建)。
. "$ROOT/scripts/smoke_fresh_bin.sh"
ensure_fresh_ouro_bin "$OURO_IDE_SMOKE_BIN" || fail "unable to build fresh ouro binary"

assert_absolute_path "IDE smoke binary" "$OURO_IDE_SMOKE_BIN"
assert_no_symlink_path_components "IDE smoke binary" "$OURO_IDE_SMOKE_BIN"
assert_not_symlink_file "IDE smoke binary" "$OURO_IDE_SMOKE_BIN"
[ -x "$OURO_IDE_SMOKE_BIN" ] || fail "IDE smoke binary is not executable: $OURO_IDE_SMOKE_BIN"

assert_absolute_path "IDE smoke log dir" "$OURO_IDE_SMOKE_LOG_DIR"
assert_no_symlink_path_components "IDE smoke log dir" "$OURO_IDE_SMOKE_LOG_DIR"
assert_not_symlink_dir "IDE smoke log dir" "$OURO_IDE_SMOKE_LOG_DIR"
mkdir -p "$OURO_IDE_SMOKE_LOG_DIR"
assert_no_symlink_path_components "IDE smoke log dir" "$OURO_IDE_SMOKE_LOG_DIR"
assert_not_symlink_dir "IDE smoke log dir" "$OURO_IDE_SMOKE_LOG_DIR"

workspace_root_provided=0
if [ -z "${WORKSPACE:-}" ]; then
	SMOKE_ROOT="$(mktemp -d "$TMPDIR_BASE/ouro-ide-smoke-XXXXXX")"
	WORKSPACE="$SMOKE_ROOT/workspaces"
else
	workspace_root_provided=1
fi

assert_absolute_path "IDE smoke workspace root" "$WORKSPACE"
assert_no_symlink_path_components "IDE smoke workspace root" "$WORKSPACE"
assert_not_symlink_dir "IDE smoke workspace root" "$WORKSPACE"

if [ -e "$WORKSPACE" ] && [ ! -d "$WORKSPACE" ]; then
	fail "IDE smoke workspace root exists and is not a directory"
fi

if [ "$workspace_root_provided" -eq 1 ] && [ -d "$WORKSPACE" ]; then
	existing_entry="$(find "$WORKSPACE" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null || true)"
	if [ -n "$existing_entry" ] && [ "${OURO_IDE_SMOKE_ALLOW_EXISTING_WORKSPACE:-}" != "1" ]; then
		fail "refusing existing non-empty IDE smoke workspace root; set OURO_IDE_SMOKE_ALLOW_EXISTING_WORKSPACE=1 only for a safe smoke workspace"
	fi
fi

mkdir -p "$WORKSPACE"
assert_no_symlink_path_components "IDE smoke workspace root" "$WORKSPACE"
assert_not_symlink_dir "IDE smoke workspace root" "$WORKSPACE"

DIRECT_WORKSPACE="$WORKSPACE/direct-use-workspace"
AI_WORKSPACE="$WORKSPACE/ai-provider-workspace"
DIRECT_LOG_DIR="$OURO_IDE_SMOKE_LOG_DIR/direct-use"
AI_LOG_DIR="$OURO_IDE_SMOKE_LOG_DIR/ai-provider"
DIRECT_STAGE_LOG="$OURO_IDE_SMOKE_LOG_DIR/direct-use.txt"
AI_STAGE_LOG="$OURO_IDE_SMOKE_LOG_DIR/ai-provider.txt"

cd "$ROOT"

printf 'ide-smoke: workspace-root %s\n' "$WORKSPACE"
printf 'ide-smoke: logs %s\n' "$OURO_IDE_SMOKE_LOG_DIR"
printf 'binary: %s\n' "$OURO_IDE_SMOKE_BIN"

run_stage_logged "direct-use smoke" "$DIRECT_STAGE_LOG" env \
	WORKSPACE="$DIRECT_WORKSPACE" \
	OURO_DIRECT_USE_SMOKE_BIN="$OURO_IDE_SMOKE_BIN" \
	OURO_DIRECT_USE_SMOKE_LOG_DIR="$DIRECT_LOG_DIR" \
	OURO_DIRECT_USE_SMOKE_ALLOW_EXISTING_WORKSPACE="${OURO_IDE_SMOKE_ALLOW_EXISTING_WORKSPACE:-}" \
	"$TRUSTED_SHELL" "$ROOT/scripts/smoke_direct_use.sh"
grep -q 'direct-use-smoke: ok' "$DIRECT_STAGE_LOG" || fail "direct-use smoke did not report ok"
printf 'direct-use: ok\n'

run_stage_logged "AI provider smoke" "$AI_STAGE_LOG" env \
	WORKSPACE="$AI_WORKSPACE" \
	OURO_AI_PROVIDER_SMOKE_BIN="$OURO_IDE_SMOKE_BIN" \
	OURO_AI_PROVIDER_SMOKE_LOG_DIR="$AI_LOG_DIR" \
	OURO_AI_PROVIDER_SMOKE_ALLOW_EXISTING_WORKSPACE="${OURO_IDE_SMOKE_ALLOW_EXISTING_WORKSPACE:-}" \
	OURO_AI_PROVIDER_SMOKE_SKIP_FAKE_SERVER="${OURO_AI_PROVIDER_SMOKE_SKIP_FAKE_SERVER:-}" \
	"$TRUSTED_SHELL" "$ROOT/scripts/smoke_ai_provider.sh"
grep -q 'ai-provider-smoke: ok' "$AI_STAGE_LOG" || fail "AI provider smoke did not report ok"
grep -F -q 'smoke-secret-value' "$AI_STAGE_LOG" && fail "AI provider smoke output leaked API key value"
printf 'ai-provider: ok\n'

printf 'ide-smoke: ok\n'
