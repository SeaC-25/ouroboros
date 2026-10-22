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
BUNDLE_NAME="${OURO_RELEASE_MANAGED_SMOKE_BUNDLE:-nuclei-templates}"
PACK_NAME="${OURO_RELEASE_MANAGED_SMOKE_PACK:-web-api-pack}"

if [ -z "${OURO_RELEASE_MANAGED_SMOKE_LOG_DIR:-}" ]; then
	OURO_RELEASE_MANAGED_SMOKE_LOG_DIR="$(mktemp -d "$TMPDIR_BASE/ouro-release-managed-smoke-logs-XXXXXX")"
fi

mkdir -p "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR"

fail() {
	printf 'release-managed-smoke: failed: %s\n' "$1" >&2
	printf 'release-managed-smoke: logs %s\n' "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR" >&2
	exit 1
}

workspace_provided=0
if [ -z "${WORKSPACE:-}" ]; then
	SMOKE_ROOT="$(mktemp -d "$TMPDIR_BASE/ouro-release-managed-smoke-XXXXXX")"
	WORKSPACE="$SMOKE_ROOT/workspace"
else
	workspace_provided=1
fi

if [ -e "$WORKSPACE" ] && [ ! -d "$WORKSPACE" ]; then
	fail "workspace path exists and is not a directory"
fi

if [ "$workspace_provided" -eq 1 ] && [ -d "$WORKSPACE" ]; then
	existing_entry="$(find "$WORKSPACE" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null || true)"
	if [ -n "$existing_entry" ] && [ "${OURO_RELEASE_MANAGED_SMOKE_ALLOW_EXISTING_WORKSPACE:-}" != "1" ]; then
		fail "refusing existing non-empty workspace; set OURO_RELEASE_MANAGED_SMOKE_ALLOW_EXISTING_WORKSPACE=1 only for a safe smoke workspace"
	fi
fi

if [ -z "${DIST_DIR:-}" ]; then
	DIST_DIR="$(mktemp -d "$TMPDIR_BASE/ouro-release-managed-smoke-dist-XXXXXX")"
fi
if [ -z "${PREFIX:-}" ]; then
	PREFIX="$(mktemp -d "$TMPDIR_BASE/ouro-release-managed-smoke-prefix-XXXXXX")"
fi

VERSION="${VERSION:-0.1.0-release-managed-smoke}"
GOOS_VALUE="$(go env GOOS)"
GOARCH_VALUE="$(go env GOARCH)"
PACKAGE="ouro_${VERSION}_${GOOS_VALUE}_${GOARCH_VALUE}"
ARCHIVE="$DIST_DIR/$PACKAGE.tar.gz"
# Fresh-binary note: scripts/release.sh unconditionally rebuilds from source
# below, so $PREFIX/bin/ouro can never be a stale prebuilt artifact.
OURO="$PREFIX/bin/ouro"
RELEASE_ROOT="$PREFIX/share/ouroboros"

run_logged() {
	label="$1"
	log_path="$2"
	shift 2
	if ! "$@" > "$log_path" 2>&1; then
		fail "$label failed; see $log_path"
	fi
}

run_expect_fail_logged() {
	label="$1"
	log_path="$2"
	shift 2
	if "$@" > "$log_path" 2>&1; then
		fail "$label unexpectedly succeeded; see $log_path"
	fi
}

assert_log_contains() {
	log_path="$1"
	needle="$2"
	message="$3"
	grep -F -q "$needle" "$log_path" || fail "$message"
}

cd "$ROOT"

printf 'release-managed-smoke: workspace %s\n' "$WORKSPACE"
printf 'release-managed-smoke: release-root %s\n' "$RELEASE_ROOT"
printf 'release-managed-smoke: logs %s\n' "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR"

run_logged "release" "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release.txt" \
	env GOPROXY="$GOPROXY" VERSION="$VERSION" DIST_DIR="$DIST_DIR" GOOS="$GOOS_VALUE" GOARCH="$GOARCH_VALUE" "$ROOT/scripts/release.sh"
[ -f "$ARCHIVE" ] || fail "release archive was not written: $ARCHIVE"
printf 'release: ok\n'

run_logged "install" "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/install.txt" \
	env PREFIX="$PREFIX" "$ROOT/scripts/install.sh" "$ARCHIVE"
[ -x "$OURO" ] || fail "installed ouro binary is missing or not executable: $OURO"
[ -d "$RELEASE_ROOT/toolchains/managed-bundles/$BUNDLE_NAME" ] || fail "installed release-managed bundle is missing: $BUNDLE_NAME"
printf 'install: ok\n'

run_logged "init" "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/init.txt" "$OURO" init "$WORKSPACE"
run_logged "strict doctor" "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/doctor.txt" "$OURO" doctor "$WORKSPACE" --require-safety-authorization
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/doctor.txt" "workspace: ready" "strict doctor did not report ready"
printf 'doctor: ready\n'

run_logged "release-managed-bundles" "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-bundles.txt" \
	"$OURO" toolchain release-managed-bundles "$RELEASE_ROOT" "$BUNDLE_NAME"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-bundles.txt" "release-managed-bundle: name=$BUNDLE_NAME status=available" "release-managed bundle inventory did not show an available bundle"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-bundles.txt" "sha256=" "release-managed bundle inventory did not expose payload digest metadata"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-bundles.txt" "corpus_status=recorded" "release-managed bundle inventory did not expose recorded corpus metadata"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-bundles.txt" "corpus_name=$BUNDLE_NAME" "release-managed bundle inventory did not expose corpus name"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-bundles.txt" "corpus_source=https://github.com/projectdiscovery/nuclei-templates" "release-managed bundle inventory did not expose corpus source"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-bundles.txt" "corpus_retrieved_at=2026-06-05T00:00:00Z" "release-managed bundle inventory did not expose corpus retrieval time"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-bundles.txt" "corpus_digest_status=matched" "release-managed bundle inventory did not match corpus digest metadata"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-bundles.txt" "corpus_file_count=1" "release-managed bundle inventory did not expose corpus file count"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-bundles.txt" "corpus_digest_algorithm=sha256" "release-managed bundle inventory did not expose corpus digest algorithm"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-bundles.txt" "corpus_digest_scope=directory_canonical_records" "release-managed bundle inventory did not expose corpus digest scope"
BUNDLE_SHA="$(awk '
	{
		for (i = 1; i <= NF; i++) {
			if ($i ~ /^sha256=/) {
				sub(/^sha256=/, "", $i)
				print $i
				exit
			}
		}
	}
' "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-bundles.txt")"
[ -n "$BUNDLE_SHA" ] || fail "could not extract release-managed bundle sha256"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-bundles.txt" "corpus_sha256=$BUNDLE_SHA" "release-managed bundle inventory corpus digest did not match payload digest"
printf 'release-managed-bundles: %s\n' "$BUNDLE_NAME"

run_logged "release-managed-status before install" "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-before.txt" \
	"$OURO" toolchain release-managed-status "$WORKSPACE" "$RELEASE_ROOT" "$BUNDLE_NAME" --expected-sha256 "$BUNDLE_SHA"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-before.txt" "digest_status=verified" "release-managed status did not verify digest before install"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-before.txt" "digest_scope=directory_canonical_records" "release-managed status did not report rule-bundle directory digest scope"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-before.txt" "expected_sha256_status=matched" "release-managed status did not match expected digest before install"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-before.txt" "import_status=not_imported" "release-managed status did not report not_imported before install"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-before.txt" "link_status=missing" "release-managed status did not report missing link before install"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-before.txt" "update_status=current" "release-managed status did not expose current update metadata"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-before.txt" "corpus_status=recorded" "release-managed status did not expose recorded corpus metadata"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-before.txt" "corpus_name=$BUNDLE_NAME" "release-managed status did not expose corpus name before install"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-before.txt" "corpus_digest_status=matched" "release-managed status did not match corpus digest metadata before install"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-before.txt" "corpus_sha256=$BUNDLE_SHA" "release-managed status corpus digest did not match payload digest before install"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-before.txt" "corpus_source=https://github.com/projectdiscovery/nuclei-templates" "release-managed status did not expose corpus source before install"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-before.txt" "corpus_digest_scope=directory_canonical_records" "release-managed status did not expose corpus digest scope before install"
printf 'release-managed-status: import_status=not_imported link_status=missing expected_sha256_status=matched\n'

run_logged "release-managed-status-source set" "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-source-set.txt" \
	"$OURO" toolchain release-managed-status-source-set "$WORKSPACE" "$RELEASE_ROOT" --set --pin-bundle "$BUNDLE_NAME" --expected-sha256 "$BUNDLE_SHA" --set-by smoke --reason "release managed smoke"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-source-set.txt" "release-managed-status-source: status=current" "release-managed status source set did not report current"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-source-set.txt" "expected_sha256_status=matched" "release-managed status source set did not match expected digest"

run_logged "release-managed-status-source read" "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-source.txt" \
	"$OURO" toolchain release-managed-status-source "$WORKSPACE"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-source.txt" "release-managed-status-source: status=current" "release-managed status source read did not report current"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-source.txt" "expected_sha256_bundle=$BUNDLE_NAME" "release-managed status source did not preserve pinned bundle"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-source.txt" "expected_sha256=$BUNDLE_SHA" "release-managed status source did not preserve expected digest"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-source.txt" "expected_sha256_status=matched" "release-managed status source read did not match expected digest"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-source.txt" "available=1" "release-managed status source did not count the available bundle"
printf 'release-managed-status-source: status=current expected_sha256_status=matched\n'

run_logged "release-managed-install-plan" "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install-plan.txt" \
	"$OURO" toolchain release-managed-install-plan "$WORKSPACE" "$RELEASE_ROOT" "$PACK_NAME" "$BUNDLE_NAME" --expected-sha256 "$BUNDLE_SHA"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install-plan.txt" "release-managed-install-plan: status=ready" "release-managed install plan did not report ready"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install-plan.txt" "mode=metadata-only" "release-managed install plan did not stay metadata-only"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install-plan.txt" "action=copy-and-link" "release-managed install plan did not report copy-and-link"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install-plan.txt" "expected_sha256_status=matched" "release-managed install plan did not match expected digest"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install-plan.txt" "corpus_status=recorded" "release-managed install plan did not expose corpus metadata"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install-plan.txt" "corpus_digest_status=matched" "release-managed install plan did not match corpus digest metadata"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install-plan.txt" "corpus_sha256=$BUNDLE_SHA" "release-managed install plan corpus digest did not match payload digest"
[ ! -e "$WORKSPACE/toolchain/managed-bundles/$BUNDLE_NAME" ] || fail "release-managed install plan copied workspace bundle"
[ ! -e "$WORKSPACE/toolchain/rules/$BUNDLE_NAME" ] && [ ! -L "$WORKSPACE/toolchain/rules/$BUNDLE_NAME" ] || fail "release-managed install plan created rule link"
printf 'release-managed-install-plan: status=ready\n'

run_expect_fail_logged "release-managed-install without execute" "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install-without-execute.txt" \
	"$OURO" toolchain release-managed-install "$WORKSPACE" "$RELEASE_ROOT" "$PACK_NAME" "$BUNDLE_NAME" --expected-sha256 "$BUNDLE_SHA"
[ ! -e "$WORKSPACE/toolchain/managed-bundles/$BUNDLE_NAME" ] || fail "release-managed install without --execute copied workspace bundle"
[ ! -e "$WORKSPACE/toolchain/rules/$BUNDLE_NAME" ] && [ ! -L "$WORKSPACE/toolchain/rules/$BUNDLE_NAME" ] || fail "release-managed install without --execute created rule link"
printf 'release-managed-install-without-execute: rejected\n'

run_logged "release-managed-install" "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install.txt" \
	"$OURO" toolchain release-managed-install "$WORKSPACE" "$RELEASE_ROOT" "$PACK_NAME" "$BUNDLE_NAME" --expected-sha256 "$BUNDLE_SHA" --execute
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install.txt" "release-managed-install: status=installed" "release-managed install did not report installed"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install.txt" "mode=workspace-copy-link" "release-managed install did not use workspace copy/link mode"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install.txt" "expected_sha256_status=matched" "release-managed install did not match expected digest"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install.txt" "corpus_status=recorded" "release-managed install did not preserve corpus metadata"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install.txt" "corpus_digest_status=matched" "release-managed install did not preserve matched corpus digest metadata"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-install.txt" "corpus_sha256=$BUNDLE_SHA" "release-managed install did not preserve corpus digest metadata"
[ -f "$WORKSPACE/toolchain/managed-bundles/$BUNDLE_NAME/payload/http/ouro-default-healthcheck.yaml" ] || fail "installed managed rule payload is missing"
[ -L "$WORKSPACE/toolchain/rules/$BUNDLE_NAME" ] || fail "managed rule bundle link is missing"
printf 'release-managed-install: status=installed expected_sha256_status=matched\n'

run_logged "release-managed-status after install" "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-after.txt" \
	"$OURO" toolchain release-managed-status "$WORKSPACE" "$RELEASE_ROOT" "$BUNDLE_NAME" --expected-sha256 "$BUNDLE_SHA"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-after.txt" "expected_sha256_status=matched" "release-managed status did not match expected digest after install"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-after.txt" "import_status=linked" "release-managed status did not report linked import after install"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-after.txt" "link_status=linked" "release-managed status did not report linked final path after install"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-after.txt" "corpus_status=recorded" "release-managed status did not preserve corpus metadata after install"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-after.txt" "corpus_digest_status=matched" "release-managed status did not preserve corpus digest metadata after install"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/release-managed-status-after.txt" "corpus_sha256=$BUNDLE_SHA" "release-managed status did not preserve corpus digest after install"
printf 'release-managed-status: import_status=linked link_status=linked\n'

run_logged "run --tui" "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/run-tui.txt" "$OURO" run "$WORKSPACE" --tui
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/run-tui.txt" "Workspace" "run --tui did not include Workspace panel"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/run-tui.txt" "Runtime Tabs" "run --tui did not include Runtime Tabs panel"
# 期望模型取自工作区 providers.yaml 中第一个 enabled 的 provider,
# 而非硬编码 offline-model:init 时生成默认配置(web-ai-media-editor 或 offline),
# 两者都应能通过本断言。
EXPECTED_MODEL=$(awk '
  /^[[:space:]]*- name:/ { if (seen && enabled && model != "") { print model; exit } seen=1; enabled=0; model="" }
  /^[[:space:]]*model:[[:space:]]*/ { sub(/^[[:space:]]*model:[[:space:]]*/, ""); model=$0 }
  /^[[:space:]]*enabled:[[:space:]]*true/ { enabled=1 }
  END { if (seen && enabled && model != "") print model }
' "$WORKSPACE/providers.yaml")
[ -n "$EXPECTED_MODEL" ] || fail "could not derive expected model from workspace providers.yaml"
assert_log_contains "$OURO_RELEASE_MANAGED_SMOKE_LOG_DIR/run-tui.txt" "model:$EXPECTED_MODEL" "run --tui did not report the enabled provider model from providers.yaml"
printf 'tui: ok\n'

printf 'release-managed-smoke: ok\n'
