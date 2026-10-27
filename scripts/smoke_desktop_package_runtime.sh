#!/bin/sh
set -eu

case "$0" in
	*/*) ROOT_DIR="${0%/*}" ;;
	*) ROOT_DIR="." ;;
esac
ROOT="$(CDPATH= cd -- "$ROOT_DIR/.." && pwd)"
# The Go packager names the output dir Ouroboros-{GOOS}-{GOARCH}
# (windows-amd64); translate Node's win32/x64 platform names to match.
PLATFORM_DIR="$(node -p 'const p = process.platform === "win32" ? "windows" : process.platform; const a = process.arch === "x64" ? "amd64" : process.arch; p + "-" + a')"
PACKAGE_DIR="${OURO_DESKTOP_PACKAGE_DIR:-$ROOT/gui/dist/desktop/Ouroboros-$PLATFORM_DIR}"
TMPDIR_BASE="${TMPDIR:-/tmp}"
SMOKE_ROOT="$(mktemp -d "$TMPDIR_BASE/ouro-desktop-package-runtime-smoke-XXXXXX")"
WORKSPACE="$SMOKE_ROOT/workspace"
HOME_DIR="$SMOKE_ROOT/home"
USER_DATA="$SMOKE_ROOT/userData"
XDG_CONFIG="$SMOKE_ROOT/xdg-config"
XDG_DATA="$SMOKE_ROOT/xdg-data"
XDG_CACHE="$SMOKE_ROOT/xdg-cache"
CDP_PORT="${OURO_DESKTOP_PACKAGE_RUNTIME_CDP_PORT:-19224}"
APP_LOG="$SMOKE_ROOT/electron.log"
RUNTIME_OUT="$SMOKE_ROOT/runtime.json"
DOM_OUT="$SMOKE_ROOT/dom.txt"

fail() {
  printf 'desktop-package-runtime-smoke: 失败：%s\n' "$1" >&2
  printf 'desktop-package-runtime-smoke: 临时目录 %s\n' "$SMOKE_ROOT" >&2
  if [ -f "$APP_LOG" ]; then
    printf 'desktop-package-runtime-smoke: Electron 日志尾部如下\n' >&2
    tail -80 "$APP_LOG" >&2 || true
  fi
  exit 1
}

. "$ROOT/scripts/lib_smoke_common.sh"

cleanup() {
  if [ -n "${APP_PID:-}" ]; then
    cleanup_tree "$APP_PID"
  fi
}
trap cleanup EXIT INT TERM

assert_file() {
  label="$1"
  path="$2"
  [ -f "$path" ] || fail "$label 缺失：$path"
}

assert_executable() {
  label="$1"
  path="$2"
  assert_file "$label" "$path"
  [ -x "$path" ] || fail "$label 不可执行：$path"
}


case "$(uname -s)" in
  Linux*)
    if [ -x "$PACKAGE_DIR/Ouroboros" ]; then
      APP_BINARY="$PACKAGE_DIR/Ouroboros"
    elif [ -x "$PACKAGE_DIR/ouroboros" ]; then
      APP_BINARY="$PACKAGE_DIR/ouroboros"
    else
      APP_BINARY="$PACKAGE_DIR/Ouroboros"
    fi
    ;;
  Darwin*) APP_BINARY="$PACKAGE_DIR/Ouroboros.app/Contents/MacOS/Ouroboros" ;;
  MINGW*|MSYS*|CYGWIN*) APP_BINARY="$PACKAGE_DIR/Ouroboros.exe" ;;
  *) APP_BINARY="$PACKAGE_DIR/Ouroboros" ;;
esac

BACKEND_BINARY="$PACKAGE_DIR/resources/ouro"
if [ "$(node -p 'process.platform')" = "win32" ]; then
  BACKEND_BINARY="$PACKAGE_DIR/resources/ouro.exe"
fi

assert_executable "打包后的 Electron app binary" "$APP_BINARY"
assert_port_free "$CDP_PORT" "Electron DevTools"
assert_executable "打包后的 backend sidecar" "$BACKEND_BINARY"
assert_file "打包后的 preload" "$PACKAGE_DIR/resources/app/dist/main/preload.js"
assert_file "打包后的 renderer" "$PACKAGE_DIR/resources/app/dist/renderer/index.html"

mkdir -p \
  "$HOME_DIR" \
  "$USER_DATA" \
  "$XDG_CONFIG" \
  "$XDG_DATA" \
  "$XDG_CACHE" \
  "$WORKSPACE/replay" \
  "$WORKSPACE/toolchain/runtime-pack" \
  "$WORKSPACE/artifacts" \
  "$WORKSPACE/evidence" \
  "$WORKSPACE/attempts" \
  "$WORKSPACE/findings" \
  "$WORKSPACE/reports" \
  "$WORKSPACE/runtime"

cat > "$WORKSPACE/workspace.yaml" <<'YAML'
version: 1
workspace:
  name: 真实打包运行时工作区
  description: 由 packaged Electron runtime smoke 创建
  operator: 桌面运行时验证
  tags:
    - desktop
    - runtime
YAML
cat > "$WORKSPACE/mission.yaml" <<'YAML'
version: 1
mission:
  name: 打包运行时真实任务
  goal: 通过真实 Electron preload bridge 读取 backend
  command: printf 'packaged runtime smoke\n'
YAML
cat > "$WORKSPACE/scope.yaml" <<'YAML'
version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
YAML
cat > "$WORKSPACE/authorization.yaml" <<'YAML'
version: 1
safety_bypasses: []
YAML
cat > "$WORKSPACE/providers.yaml" <<'YAML'
version: 1
routing:
  strategy: enabled_order_failover
providers:
  - name: runtime-local
    type: openai_compatible
    base_url: https://runtime-provider.local/v1
    model: packaged-runtime-model
    enabled: true
YAML
cat > "$WORKSPACE/toolchain/runtime-pack/manifest.yaml" <<'YAML'
name: runtime-pack
version: "0.1.0"
capabilities:
  - runtime.smoke
runner: native
parser: runtime_parser
healthcheck: built_in
dependencies:
  - name: sh
    kind: binary
    optional: true
YAML
printf '# 打包运行时报告\n\n来自 packaged Electron runtime smoke。\n' > "$WORKSPACE/reports/runtime.md"
printf '{"title":"运行时真实发现","severity":"info","endpoint":"https://runtime-smoke.local"}\n' > "$WORKSPACE/findings/runtime.json"

if [ "$(uname -s)" = "Linux" ] && [ -z "${DISPLAY:-}" ]; then
  if command -v xvfb-run >/dev/null 2>&1; then
    env \
      HOME="$HOME_DIR" \
      XDG_CONFIG_HOME="$XDG_CONFIG" \
      XDG_DATA_HOME="$XDG_DATA" \
      XDG_CACHE_HOME="$XDG_CACHE" \
      OUROBOROS_WORKSPACE="$WORKSPACE" \
      ELECTRON_ENABLE_LOGGING=1 \
      xvfb-run -a -s "-screen 0 1440x1000x24" \
      "$APP_BINARY" \
      --no-sandbox \
      --disable-gpu \
      --ouro-desktop-memory-profile=low \
      "--js-flags=--max-old-space-size=256 --max-semi-space-size=8" \
      --renderer-process-limit=1 \
      --user-data-dir="$USER_DATA" \
      --remote-debugging-port="$CDP_PORT" \
      > "$APP_LOG" 2>&1 &
    APP_PID=$!
  else
    fail "Linux 环境没有 DISPLAY，且找不到 xvfb-run；请安装 xvfb 或提供可用 DISPLAY"
  fi
else
  env \
    HOME="$HOME_DIR" \
    XDG_CONFIG_HOME="$XDG_CONFIG" \
    XDG_DATA_HOME="$XDG_DATA" \
    XDG_CACHE_HOME="$XDG_CACHE" \
    OUROBOROS_WORKSPACE="$WORKSPACE" \
    ELECTRON_ENABLE_LOGGING=1 \
    "$APP_BINARY" \
    --no-sandbox \
    --disable-gpu \
    --ouro-desktop-memory-profile=low \
    "--js-flags=--max-old-space-size=256 --max-semi-space-size=8" \
    --renderer-process-limit=1 \
    --user-data-dir="$USER_DATA" \
    --remote-debugging-port="$CDP_PORT" \
    > "$APP_LOG" 2>&1 &
  APP_PID=$!
fi

WATCH_PID="$APP_PID"
wait_http "http://127.0.0.1:$CDP_PORT/json/version" "Electron DevTools"

node - "$CDP_PORT" "$RUNTIME_OUT" "$DOM_OUT" "$WORKSPACE" "${OURO_DESKTOP_PACKAGE_RUNTIME_WS_MODULE:-}" <<'NODE' || fail "renderer/preload/backend runtime 检查失败"
const [cdpPort, runtimeOut, domOut, workspacePath, webSocketModule] = process.argv.slice(2);
const fs = await import('node:fs/promises');
const WebSocket = globalThis.WebSocket || (webSocketModule
  ? (await import(new URL(`file:///${webSocketModule.replace(/\\/g, '/')}`))).default
  : null);

if (!WebSocket) {
  throw new Error('Node WebSocket implementation is unavailable');
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function findPage() {
  let pages = [];
  for (let i = 0; i < 120; i += 1) {
    pages = await fetch(`http://127.0.0.1:${cdpPort}/json/list`).then(response => response.json());
    const page = pages.find(item => item.type === 'page' && String(item.url || '').includes('index.html'))
      || pages.find(item => item.type === 'page')
      || pages[0];
    if (page?.webSocketDebuggerUrl) {
      return page;
    }
    await sleep(250);
  }
  throw new Error(`Electron renderer page websocket is unavailable: ${JSON.stringify(pages)}`);
}

const page = await findPage();
const socket = new WebSocket(page.webSocketDebuggerUrl);
let nextID = 1;
const pending = new Map();

socket.addEventListener('message', event => {
  const message = JSON.parse(event.data);
  if (message.id && pending.has(message.id)) {
    const { resolve, reject } = pending.get(message.id);
    pending.delete(message.id);
    if (message.error) {
      reject(new Error(message.error.message || JSON.stringify(message.error)));
    } else {
      resolve(message.result);
    }
  }
});

await new Promise((resolve, reject) => {
  socket.addEventListener('open', resolve, { once: true });
  socket.addEventListener('error', () => reject(new Error('Electron renderer websocket error')), { once: true });
});

function send(method, params = {}) {
  const id = nextID++;
  socket.send(JSON.stringify({ id, method, params }));
  return new Promise((resolve, reject) => pending.set(id, { resolve, reject }));
}

async function evaluate(expression) {
  const result = await send('Runtime.evaluate', {
    expression,
    awaitPromise: true,
    returnByValue: true,
  });
  if (result.exceptionDetails) {
    throw new Error(result.exceptionDetails.text || JSON.stringify(result.exceptionDetails));
  }
  return result.result?.value;
}

let last = null;
for (let i = 0; i < 160; i += 1) {
  last = await evaluate(`
    (async () => {
      const bodyText = document.body ? document.body.innerText : "";
      const bridge = window.ouroboros;
      const bridgeKeys = bridge && typeof bridge === 'object' ? Object.keys(bridge).sort() : [];
      const domReady = document.readyState === 'interactive' || document.readyState === 'complete';
      if (!bridge) {
        return { ready: false, reason: 'preload bridge missing', domReady, bridgeKeys, bodyText: bodyText.slice(0, 1200) };
      }
      if (typeof bridge.getAPIBaseURL !== 'function' || typeof bridge.backendStatus !== 'function') {
        return { ready: false, reason: 'preload bridge methods missing', domReady, bridgeKeys, bodyText: bodyText.slice(0, 1200) };
      }
      let status = {};
      let apiBaseURL = '';
      try {
        status = await bridge.backendStatus();
        apiBaseURL = await bridge.getAPIBaseURL();
      } catch (error) {
        return { ready: false, reason: String(error && error.message ? error.message : error), domReady, bridgeKeys, bodyText: bodyText.slice(0, 1200) };
      }
      let healthOk = false;
      let bootstrap = {};
      let fetchError = '';
      if (apiBaseURL) {
        try {
          const health = await fetch(apiBaseURL + '/api/health', { cache: 'no-store' });
          healthOk = health.ok;
          const bootstrapResponse = await fetch(apiBaseURL + '/api/gui/bootstrap', { cache: 'no-store' });
          if (bootstrapResponse.ok) {
            bootstrap = await bootstrapResponse.json();
          } else {
            fetchError = 'bootstrap status ' + bootstrapResponse.status;
          }
        } catch (error) {
          fetchError = String(error && error.message ? error.message : error);
        }
      }
      const workspaceName = bootstrap && bootstrap.workspace ? bootstrap.workspace.name : '';
      const providerModel = bootstrap && bootstrap.provider ? bootstrap.provider.model : '';
      const missionName = bootstrap && bootstrap.mission ? bootstrap.mission.name : '';
      const workspacePath = bootstrap && bootstrap.workspace ? bootstrap.workspace.path : '';
      const ready = Boolean(
        domReady
          && status
          && status.status === 'ready'
          && status.running === true
          && apiBaseURL
          && healthOk
          && workspaceName === '真实打包运行时工作区'
          && providerModel === 'packaged-runtime-model'
          && missionName === '打包运行时真实任务'
          && workspacePath === ${JSON.stringify(workspacePath)}
      );
      return {
        ready,
        reason: ready ? 'ready' : 'waiting for Electron-managed backend and renderer data',
        domReady,
        bridgeKeys,
        status,
        apiBaseURL,
        healthOk,
        fetchError,
        workspaceName,
        providerModel,
        missionName,
        workspacePath,
        bodyText: bodyText.slice(0, 2000),
      };
    })()
  `);
  if (last?.ready) {
    await fs.writeFile(runtimeOut, JSON.stringify(last, null, 2) + '\n');
    await fs.writeFile(domOut, `${last.bodyText || ''}\n`);
    void evaluate(`window.ouroboros?.closeWindow?.()`).catch(() => {});
    socket.close();
    process.exit(0);
  }
  await sleep(250);
}

await fs.writeFile(runtimeOut, JSON.stringify(last, null, 2) + '\n');
await fs.writeFile(domOut, `${last?.bodyText || ''}\n`);
throw new Error(`runtime did not become ready: ${JSON.stringify(last)}`);
NODE

printf 'desktop-package-runtime-smoke: 通过\n'
printf 'desktop-package-runtime-smoke: app %s\n' "$APP_BINARY"
printf 'desktop-package-runtime-smoke: workspace %s\n' "$WORKSPACE"
printf 'desktop-package-runtime-smoke: 真实 preload bridge/backend 结果 %s\n' "$RUNTIME_OUT"
printf 'desktop-package-runtime-smoke: 临时目录 %s\n' "$SMOKE_ROOT"
