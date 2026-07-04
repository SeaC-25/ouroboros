#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
PACKAGE_DIR="${OURO_DESKTOP_PACKAGE_DIR:-$ROOT/gui/dist/desktop/Ouroboros-$(node -p 'process.platform + "-" + process.arch')}"
TMPDIR_BASE="${TMPDIR:-/tmp}"
SMOKE_ROOT="$(mktemp -d "$TMPDIR_BASE/ouro-desktop-package-smoke-XXXXXX")"
WORKSPACE="$SMOKE_ROOT/workspace"
API_PORT="${OURO_DESKTOP_PACKAGE_SMOKE_API_PORT:-18082}"
API_LOG="$SMOKE_ROOT/api.log"

# This smoke checks the packaged sidecar/layout only. It proves the package
# contains the expected files and that the packaged backend sidecar can serve a
# real workspace. It does not prove that the packaged Electron runtime starts.

fail() {
  printf 'desktop-package-sidecar/layout-smoke: 失败：%s\n' "$1" >&2
  printf 'desktop-package-sidecar/layout-smoke: 临时目录 %s\n' "$SMOKE_ROOT" >&2
  exit 1
}

cleanup() {
  if [ -n "${API_PID:-}" ]; then
    kill "$API_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

assert_file() {
  label="$1"
  path="$2"
  [ -f "$path" ] || fail "$label missing: $path"
}

assert_contains() {
  label="$1"
  path="$2"
  needle="$3"
  grep -F -q "$needle" "$path" || fail "$label missing expected content: $needle"
}

assert_tree_contains() {
  label="$1"
  path="$2"
  needle="$3"
  grep -R -F -q "$needle" "$path" || fail "$label missing expected content: $needle"
}

assert_executable() {
  label="$1"
  path="$2"
  assert_file "$label" "$path"
  [ -x "$path" ] || fail "$label is not executable: $path"
}

wait_http() {
  url="$1"
  label="$2"
  i=0
  while [ "$i" -lt 80 ]; do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    i=$((i + 1))
    sleep 0.25
  done
  fail "$label did not become ready"
}

case "$(uname -s)" in
  Linux*) APP_BINARY="$PACKAGE_DIR/Ouroboros" ;;
  Darwin*) APP_BINARY="$PACKAGE_DIR/Ouroboros.app/Contents/MacOS/Ouroboros" ;;
  MINGW*|MSYS*|CYGWIN*) APP_BINARY="$PACKAGE_DIR/Ouroboros.exe" ;;
  *) APP_BINARY="$PACKAGE_DIR/Ouroboros" ;;
esac

BACKEND_BINARY="$PACKAGE_DIR/resources/ouro"
if [ "$(node -p 'process.platform')" = "win32" ]; then
  BACKEND_BINARY="$PACKAGE_DIR/resources/ouro.exe"
fi

assert_executable "Electron app binary" "$APP_BINARY"
assert_executable "packaged backend sidecar" "$BACKEND_BINARY"
assert_file "packaged app package" "$PACKAGE_DIR/resources/app/package.json"
assert_file "packaged main process" "$PACKAGE_DIR/resources/app/dist/main/main.js"
assert_file "packaged preload" "$PACKAGE_DIR/resources/app/dist/main/preload.js"
assert_file "packaged backend manager" "$PACKAGE_DIR/resources/app/dist/main/backend.js"
assert_file "packaged desktop state manager" "$PACKAGE_DIR/resources/app/dist/main/desktop-state.js"
assert_file "packaged renderer" "$PACKAGE_DIR/resources/app/dist/renderer/index.html"
assert_contains "packaged desktop menu" "$PACKAGE_DIR/resources/app/dist/main/main.js" "buildApplicationMenu"
assert_contains "packaged desktop menu bridge" "$PACKAGE_DIR/resources/app/dist/main/main.js" "desktop:menu-action"
assert_contains "packaged backend restart IPC" "$PACKAGE_DIR/resources/app/dist/main/main.js" "backend:restart"
assert_contains "packaged window minimize IPC" "$PACKAGE_DIR/resources/app/dist/main/main.js" "window:minimize"
assert_contains "packaged window maximize IPC" "$PACKAGE_DIR/resources/app/dist/main/main.js" "window:toggle-maximize"
assert_contains "packaged window close IPC" "$PACKAGE_DIR/resources/app/dist/main/main.js" "window:close"
assert_contains "packaged preload restart bridge" "$PACKAGE_DIR/resources/app/dist/main/preload.js" "restartBackend"
assert_contains "packaged preload window state bridge" "$PACKAGE_DIR/resources/app/dist/main/preload.js" "desktop:window-state"
assert_contains "packaged preload minimize bridge" "$PACKAGE_DIR/resources/app/dist/main/preload.js" "minimizeWindow"
assert_contains "packaged preload maximize bridge" "$PACKAGE_DIR/resources/app/dist/main/preload.js" "toggleMaximizeWindow"
assert_contains "packaged preload close bridge" "$PACKAGE_DIR/resources/app/dist/main/preload.js" "closeWindow"
assert_contains "packaged backend crash state" "$PACKAGE_DIR/resources/app/dist/main/backend.js" "status = 'crashed'"
assert_contains "packaged window state storage" "$PACKAGE_DIR/resources/app/dist/main/desktop-state.js" "window-state.json"
assert_tree_contains "packaged Chinese backend crash UI" "$PACKAGE_DIR/resources/app/dist/renderer" "桌面后端已崩溃"
assert_tree_contains "packaged Chinese backend restart UI" "$PACKAGE_DIR/resources/app/dist/renderer" "重启后端"
assert_tree_contains "packaged Chinese window controls" "$PACKAGE_DIR/resources/app/dist/renderer" "最小化窗口"
assert_tree_contains "packaged Chinese window controls" "$PACKAGE_DIR/resources/app/dist/renderer" "最大化或还原窗口"
assert_tree_contains "packaged Chinese window controls" "$PACKAGE_DIR/resources/app/dist/renderer" "关闭窗口"

mkdir -p "$WORKSPACE/replay" "$WORKSPACE/toolchain/package-pack"
cat > "$WORKSPACE/providers.yaml" <<'YAML'
version: 1
providers:
  - name: local
    model: packaged-smoke-model
    enabled: true
YAML
cat > "$WORKSPACE/toolchain/package-pack/manifest.yaml" <<'YAML'
name: package-pack
version: "0.1.0"
capabilities:
  - package.smoke
runner: native
parser: package_parser
healthcheck: built_in
dependencies:
  - name: sh
    kind: binary
    optional: true
YAML

"$BACKEND_BINARY" serve "$WORKSPACE" --port "$API_PORT" > "$API_LOG" 2>&1 &
API_PID=$!
wait_http "http://127.0.0.1:$API_PORT/api/health" "packaged backend"
curl -fsS "http://127.0.0.1:$API_PORT/api/gui/bootstrap" > "$SMOKE_ROOT/bootstrap.json"
grep -F -q 'packaged-smoke-model' "$SMOKE_ROOT/bootstrap.json" || fail "bootstrap did not use packaged smoke workspace"
grep -F -q 'package-pack' "$SMOKE_ROOT/bootstrap.json" || fail "bootstrap did not project packaged smoke toolchain"

printf 'desktop-package-sidecar/layout-smoke: 通过\n'
printf 'desktop-package-sidecar/layout-smoke: package %s\n' "$PACKAGE_DIR"
printf 'desktop-package-sidecar/layout-smoke: 临时目录 %s\n' "$SMOKE_ROOT"
printf 'desktop-package-sidecar/layout-smoke: 说明：这里只验证 package layout 和 packaged backend sidecar；Electron runtime 请运行 scripts/smoke_desktop_package_runtime.sh\n'
