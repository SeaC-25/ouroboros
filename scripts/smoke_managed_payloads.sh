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

if [ -z "${OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR:-}" ]; then
	OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR="$(mktemp -d "$TMPDIR_BASE/ouro-managed-payloads-smoke-logs-XXXXXX")"
fi

fail() {
	printf 'managed-payloads-smoke: failed: %s\n' "$1" >&2
	printf 'managed-payloads-smoke: logs %s\n' "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR" >&2
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

assert_absolute_path "managed payloads log dir" "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR"
assert_not_symlink_dir "managed payloads log dir" "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR"
mkdir -p "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR"
assert_not_symlink_dir "managed payloads log dir" "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR"

workspace_provided=0
if [ -z "${WORKSPACE:-}" ]; then
	SMOKE_ROOT="$(mktemp -d "$TMPDIR_BASE/ouro-managed-payloads-smoke-XXXXXX")"
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
	if [ -n "$existing_entry" ] && [ "${OURO_MANAGED_PAYLOADS_SMOKE_ALLOW_EXISTING_WORKSPACE:-}" != "1" ]; then
		fail "refusing existing non-empty workspace; set OURO_MANAGED_PAYLOADS_SMOKE_ALLOW_EXISTING_WORKSPACE=1 only for a safe smoke workspace"
	fi
fi

BINARY_NAME="${OURO_MANAGED_BINARY_NAME:-httpx}"
BINARY_PACK="${OURO_MANAGED_BINARY_PACK:-web-api-pack}"
BINARY_VERSION="${OURO_MANAGED_BINARY_VERSION:-operator-local}"
BINARY_SOURCE="${OURO_MANAGED_BINARY_SOURCE:-operator-local/$BINARY_NAME}"
RULE_NAME="${OURO_MANAGED_RULE_BUNDLE_NAME:-nuclei-templates}"
RULE_PACK="${OURO_MANAGED_RULE_BUNDLE_PACK:-web-api-pack}"
RULE_VERSION="${OURO_MANAGED_RULE_BUNDLE_VERSION:-operator-local}"
RULE_SOURCE="${OURO_MANAGED_RULE_BUNDLE_SOURCE:-operator-local/$RULE_NAME}"

[ -n "${OURO_MANAGED_BINARY_PAYLOAD:-}" ] || fail "OURO_MANAGED_BINARY_PAYLOAD is required"
[ -n "${OURO_MANAGED_BINARY_EXPECTED_SHA256:-}" ] || fail "OURO_MANAGED_BINARY_EXPECTED_SHA256 is required"
[ -n "${OURO_MANAGED_RULE_BUNDLE_PAYLOAD:-}" ] || fail "OURO_MANAGED_RULE_BUNDLE_PAYLOAD is required"
[ -n "${OURO_MANAGED_RULE_BUNDLE_EXPECTED_SHA256:-}" ] || fail "OURO_MANAGED_RULE_BUNDLE_EXPECTED_SHA256 is required"
assert_absolute_path "binary payload" "$OURO_MANAGED_BINARY_PAYLOAD"
assert_absolute_path "rule bundle payload" "$OURO_MANAGED_RULE_BUNDLE_PAYLOAD"
assert_not_symlink_file "binary payload" "$OURO_MANAGED_BINARY_PAYLOAD"
assert_not_symlink_file "rule bundle payload" "$OURO_MANAGED_RULE_BUNDLE_PAYLOAD"
[ -f "$OURO_MANAGED_BINARY_PAYLOAD" ] || fail "binary payload is not a regular file: $OURO_MANAGED_BINARY_PAYLOAD"
[ -d "$OURO_MANAGED_RULE_BUNDLE_PAYLOAD" ] || fail "rule bundle payload is not a directory: $OURO_MANAGED_RULE_BUNDLE_PAYLOAD"

BINARY_INSTALL="$WORKSPACE/toolchain/bin/$BINARY_NAME"
RULE_INSTALL="$WORKSPACE/toolchain/rules/$RULE_NAME"

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

run_expect_fail_ouro_logged() {
	label="$1"
	log_path="$2"
	shift 2
	if run_ouro "$@" > "$log_path" 2>&1; then
		fail "$label unexpectedly succeeded; see $log_path"
	fi
}

assert_log_contains() {
	log_path="$1"
	needle="$2"
	message="$3"
	grep -F -q "$needle" "$log_path" || fail "$message"
}

assert_path_missing() {
	path="$1"
	message="$2"
	if [ -e "$path" ] || [ -L "$path" ]; then
		fail "$message"
	fi
}

assert_link_target_under_workspace() {
	link_path="$1"
	label="$2"
	target="$(readlink "$link_path" 2>/dev/null || true)"
	[ -n "$target" ] || fail "$label install link target could not be read: $link_path"
	case "$target" in
		"$WORKSPACE"/toolchain/managed-bundles/*)
			;;
		*)
			fail "$label install link target is outside workspace managed-bundles: $target"
			;;
	esac
}

extract_field() {
	log_path="$1"
	key="$2"
	awk -v key="$key" '
		{
			for (i = 1; i <= NF; i++) {
				if ($i ~ "^" key "=") {
					sub("^" key "=", "", $i)
					print $i
					exit
				}
			}
		}
	' "$log_path"
}

cd "$ROOT"

printf 'managed-payloads-smoke: workspace %s\n' "$WORKSPACE"
printf 'managed-payloads-smoke: binary %s\n' "$BINARY_NAME"
printf 'managed-payloads-smoke: rule_bundle %s\n' "$RULE_NAME"
printf 'managed-payloads-smoke: logs %s\n' "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR"

run_ouro_logged "init" "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/init.txt" init "$WORKSPACE"
run_ouro_logged "strict doctor" "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/doctor.txt" doctor "$WORKSPACE" --require-safety-authorization
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/doctor.txt" "workspace: ready" "strict doctor did not report ready"
printf 'doctor: ready\n'

run_ouro_logged "binary managed-bundle-import" "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/binary-import.txt" \
	toolchain managed-bundle-import "$WORKSPACE" "$BINARY_NAME" "$OURO_MANAGED_BINARY_PAYLOAD" \
	--kind binary --version "$BINARY_VERSION" --source "$BINARY_SOURCE" \
	--expected-sha256 "$OURO_MANAGED_BINARY_EXPECTED_SHA256" --import
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/binary-import.txt" "managed-bundle-import: status=imported" "binary import did not report imported"
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/binary-import.txt" "kind=binary" "binary import did not report binary kind"
BINARY_SHA="$(extract_field "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/binary-import.txt" expected_sha256)"
[ -n "$BINARY_SHA" ] || fail "binary import did not expose expected_sha256"
printf 'managed-bundle-import: binary status=imported expected_sha256=%s\n' "$BINARY_SHA"

run_ouro_logged "rule bundle managed-bundle-import" "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/rule-bundle-import.txt" \
	toolchain managed-bundle-import "$WORKSPACE" "$RULE_NAME" "$OURO_MANAGED_RULE_BUNDLE_PAYLOAD" \
	--kind rule_bundle --version "$RULE_VERSION" --source "$RULE_SOURCE" \
	--expected-sha256 "$OURO_MANAGED_RULE_BUNDLE_EXPECTED_SHA256" --import
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/rule-bundle-import.txt" "managed-bundle-import: status=imported" "rule bundle import did not report imported"
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/rule-bundle-import.txt" "kind=rule_bundle" "rule bundle import did not report rule_bundle kind"
RULE_SHA="$(extract_field "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/rule-bundle-import.txt" expected_sha256)"
[ -n "$RULE_SHA" ] || fail "rule bundle import did not expose expected_sha256"
printf 'managed-bundle-import: rule_bundle status=imported expected_sha256=%s\n' "$RULE_SHA"

run_ouro_logged "binary managed-install-plan" "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/binary-install-plan.txt" \
	toolchain managed-install-plan "$WORKSPACE" "$BINARY_PACK" "$BINARY_NAME"
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/binary-install-plan.txt" "managed-install-plan: status=ready" "binary install plan did not report ready"
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/binary-install-plan.txt" "kind=binary" "binary install plan did not report binary kind"
assert_path_missing "$BINARY_INSTALL" "binary install plan created a bin link"
printf 'managed-install-plan: binary status=ready\n'

run_ouro_logged "rule bundle managed-install-plan" "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/rule-bundle-install-plan.txt" \
	toolchain managed-install-plan "$WORKSPACE" "$RULE_PACK" "$RULE_NAME"
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/rule-bundle-install-plan.txt" "managed-install-plan: status=ready" "rule bundle install plan did not report ready"
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/rule-bundle-install-plan.txt" "kind=rule_bundle" "rule bundle install plan did not report rule_bundle kind"
assert_path_missing "$RULE_INSTALL" "rule bundle install plan created a rules link"
printf 'managed-install-plan: rule_bundle status=ready\n'

run_expect_fail_ouro_logged "binary managed-install without execute" "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/binary-install-without-execute.txt" \
	toolchain managed-install "$WORKSPACE" "$BINARY_PACK" "$BINARY_NAME"
assert_path_missing "$BINARY_INSTALL" "binary managed-install without --execute created a bin link"
printf 'managed-install-without-execute: binary rejected\n'

run_expect_fail_ouro_logged "rule bundle managed-install without execute" "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/rule-bundle-install-without-execute.txt" \
	toolchain managed-install "$WORKSPACE" "$RULE_PACK" "$RULE_NAME"
assert_path_missing "$RULE_INSTALL" "rule bundle managed-install without --execute created a rules link"
printf 'managed-install-without-execute: rule_bundle rejected\n'

run_ouro_logged "binary managed-install" "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/binary-install.txt" \
	toolchain managed-install "$WORKSPACE" "$BINARY_PACK" "$BINARY_NAME" --execute
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/binary-install.txt" "managed-install: status=installed" "binary install did not report installed"
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/binary-install.txt" "kind=binary" "binary install did not report binary kind"
[ -L "$BINARY_INSTALL" ] || fail "binary managed-install did not create bin link: $BINARY_INSTALL"
assert_link_target_under_workspace "$BINARY_INSTALL" "binary"
printf 'managed-install: binary status=installed\n'

run_ouro_logged "rule bundle managed-install" "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/rule-bundle-install.txt" \
	toolchain managed-install "$WORKSPACE" "$RULE_PACK" "$RULE_NAME" --execute
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/rule-bundle-install.txt" "managed-install: status=installed" "rule bundle install did not report installed"
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/rule-bundle-install.txt" "kind=rule_bundle" "rule bundle install did not report rule_bundle kind"
[ -L "$RULE_INSTALL" ] || fail "rule bundle managed-install did not create rules link: $RULE_INSTALL"
assert_link_target_under_workspace "$RULE_INSTALL" "rule bundle"
printf 'managed-install: rule_bundle status=installed\n'

run_ouro_logged "binary managed-bundles read" "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/binary-managed-bundle.txt" \
	toolchain managed-bundles "$WORKSPACE" "$BINARY_NAME"
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/binary-managed-bundle.txt" "managed-bundle: name=$BINARY_NAME" "binary managed bundle read did not include bundle"
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/binary-managed-bundle.txt" "install=$BINARY_INSTALL" "binary managed bundle read did not expose install link"
printf 'managed-bundles: binary linked\n'

run_ouro_logged "rule bundle managed-bundles read" "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/rule-bundle-managed-bundle.txt" \
	toolchain managed-bundles "$WORKSPACE" "$RULE_NAME"
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/rule-bundle-managed-bundle.txt" "managed-bundle: name=$RULE_NAME" "rule bundle managed bundle read did not include bundle"
assert_log_contains "$OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR/rule-bundle-managed-bundle.txt" "install=$RULE_INSTALL" "rule bundle managed bundle read did not expose install link"
printf 'managed-bundles: rule_bundle linked\n'

printf 'managed-payloads-smoke: ok\n'
