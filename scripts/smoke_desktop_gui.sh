#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
TMPDIR_BASE="${TMPDIR:-/tmp}"
SMOKE_ROOT="$(mktemp -d "$TMPDIR_BASE/ouro-desktop-gui-smoke-XXXXXX")"
WORKSPACE="$SMOKE_ROOT/workspace"
API_PORT="${OURO_DESKTOP_GUI_SMOKE_API_PORT:-18081}"
PREVIEW_PORT="${OURO_DESKTOP_GUI_SMOKE_PREVIEW_PORT:-4175}"
CDP_PORT="${OURO_DESKTOP_GUI_SMOKE_CDP_PORT:-19223}"
API_LOG="$SMOKE_ROOT/api.log"
PREVIEW_LOG="$SMOKE_ROOT/preview.log"
DOM_OUT="$SMOKE_ROOT/dom.html"

fail() {
  printf 'desktop-gui-smoke: failed: %s\n' "$1" >&2
  printf 'desktop-gui-smoke: root %s\n' "$SMOKE_ROOT" >&2
  exit 1
}

cleanup() {
  if [ -n "${API_PID:-}" ]; then
    kill "$API_PID" 2>/dev/null || true
  fi
  if [ -n "${PREVIEW_PID:-}" ]; then
    kill "$PREVIEW_PID" 2>/dev/null || true
  fi
  if [ -n "${CHROMIUM_PID:-}" ]; then
    kill "$CHROMIUM_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

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

assert_contains() {
  file="$1"
  needle="$2"
  if ! grep -F -q -- "$needle" "$file"; then
    fail "missing '$needle' in $file"
  fi
}

cd "$ROOT"
mkdir -p "$WORKSPACE/replay" "$WORKSPACE/toolchain/local-pack" "$WORKSPACE/findings"
cat > "$WORKSPACE/providers.yaml" <<'YAML'
version: 1
providers:
  - name: offline
    type: offline
    model: desktop-smoke-model
    enabled: true
YAML
cat > "$WORKSPACE/toolchain/local-pack/manifest.yaml" <<'YAML'
name: local-pack
version: "0.1.0"
capabilities:
  - local.command
runner: native
parser: local_parser
healthcheck: built_in
dependencies:
  - name: sh
    kind: binary
    optional: true
YAML
printf '# Desktop GUI Smoke\n' > "$WORKSPACE/mission.md"
cat > "$WORKSPACE/findings/high.json" <<'JSON'
{"title":"真实高危发现","severity":"high","endpoint":"https://desktop-smoke.local/admin"}
JSON

GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" go run ./cmd/ouro serve "$WORKSPACE" --port "$API_PORT" > "$API_LOG" 2>&1 &
API_PID=$!
wait_http "http://127.0.0.1:$API_PORT/api/health" "api"

cat > "$SMOKE_ROOT/workspace-update.json" <<'JSON'
{
  "name": "真实桌面工作区",
  "description": "用于桌面 GUI smoke 的真实工作区配置",
  "operator": "红队工程师",
  "tags": ["desktop", "真实"]
}
JSON
curl -fsS -X PUT "http://127.0.0.1:$API_PORT/api/workspace/settings" \
  -H 'Content-Type: application/json' \
  --data-binary "@$SMOKE_ROOT/workspace-update.json" > "$SMOKE_ROOT/workspace-settings.json"
assert_contains "$SMOKE_ROOT/workspace-settings.json" "真实桌面工作区"
assert_contains "$WORKSPACE/workspace.yaml" "真实桌面工作区"
assert_contains "$WORKSPACE/workspace.yaml" "红队工程师"
assert_contains "$WORKSPACE/workspace.yaml" "desktop"

cat > "$SMOKE_ROOT/provider-update.json" <<'JSON'
{
  "name": "desktop-provider",
  "type": "openai_compatible",
  "baseUrl": "https://desktop-provider.local/v1",
  "model": "desktop-saved-model",
  "apiKeyEnv": "OURO_DESKTOP_PROVIDER_KEY",
  "enabled": true,
  "timeoutMs": 45000,
  "contextWindowTokens": 32768,
  "capabilities": ["chat", "tools"],
  "routingStrategy": "enabled_order_failover"
}
JSON
curl -fsS -X PUT "http://127.0.0.1:$API_PORT/api/provider" \
  -H 'Content-Type: application/json' \
  --data-binary "@$SMOKE_ROOT/provider-update.json" > "$SMOKE_ROOT/provider.json"
assert_contains "$SMOKE_ROOT/provider.json" "desktop-saved-model"
assert_contains "$WORKSPACE/providers.yaml" "desktop-provider"
assert_contains "$WORKSPACE/providers.yaml" "desktop-saved-model"
assert_contains "$WORKSPACE/providers.yaml" "OURO_DESKTOP_PROVIDER_KEY"
assert_contains "$WORKSPACE/providers.yaml" "enabled_order_failover"

cat > "$SMOKE_ROOT/toolchain-update.json" <<'JSON'
{
  "name": "local-pack-saved",
  "version": "0.2.0",
  "capabilities": ["local.command", "evidence.collection"],
  "runner": "command",
  "runCommand": ["sh", "-lc", "printf '工具动作日志\\n'; printf '%s\\n' \"$OURO_TOOL_ARGUMENTS\" > \"$OURO_ARTIFACT_DIR/tool-action.txt\""],
  "runTimeoutMs": 12000,
  "parser": "jsonl_parser",
  "healthcheck": "exec",
  "healthCommand": ["sh", "-c", "printf ok"]
}
JSON
curl -fsS -X PUT "http://127.0.0.1:$API_PORT/api/toolchain/packs/local" \
  -H 'Content-Type: application/json' \
  --data-binary "@$SMOKE_ROOT/toolchain-update.json" > "$SMOKE_ROOT/toolchain.json"
assert_contains "$SMOKE_ROOT/toolchain.json" "local-pack-saved"
assert_contains "$SMOKE_ROOT/toolchain.json" "jsonl_parser"
assert_contains "$WORKSPACE/toolchain/local-pack/manifest.yaml" "local-pack-saved"
assert_contains "$WORKSPACE/toolchain/local-pack/manifest.yaml" "evidence.collection"
assert_contains "$WORKSPACE/toolchain/local-pack/manifest.yaml" "runner: command"
assert_contains "$WORKSPACE/toolchain/local-pack/manifest.yaml" "run_timeout_ms: 12000"

tool_execution_id="$(node - "$API_PORT" <<'NODE'
const port = process.argv[2];

const response = await fetch(`http://127.0.0.1:${port}/api/toolchain/actions/local/run`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ arguments: '--target desktop-tool.local' }),
});
if (!response.ok) {
  throw new Error(`tool action POST returned ${response.status}: ${await response.text()}`);
}
const started = await response.json();
const executionID = started.execution?.id || '';
if (!executionID) {
  throw new Error(`tool action execution id missing: ${JSON.stringify(started)}`);
}
for (let i = 0; i < 80; i += 1) {
  const current = await fetch(`http://127.0.0.1:${port}/api/executions/${executionID}`).then(item => item.json());
  if (current.status === 'completed') {
    process.stdout.write(executionID);
    process.exit(0);
  }
  if (current.status === 'failed' || current.status === 'stopped') {
    throw new Error(`tool action execution ended with ${current.status}`);
  }
  await new Promise(resolve => setTimeout(resolve, 100));
}
throw new Error(`tool action execution did not complete: ${executionID}`);
NODE
)" || fail "tool action run smoke failed"
[ -n "$tool_execution_id" ] || fail "tool action execution id not returned"

curl -fsS "http://127.0.0.1:$API_PORT/api/executions/$tool_execution_id/logs" > "$SMOKE_ROOT/tool-action-logs.json"
curl -fsS "http://127.0.0.1:$API_PORT/api/executions/$tool_execution_id/artifacts" > "$SMOKE_ROOT/tool-action-artifacts.json"
assert_contains "$SMOKE_ROOT/tool-action-logs.json" "工具动作日志"
assert_contains "$SMOKE_ROOT/tool-action-artifacts.json" "tool-action.txt"
assert_contains "$SMOKE_ROOT/tool-action-artifacts.json" "--target desktop-tool.local"

execution_id="$(node - "$API_PORT" <<'NODE'
const port = process.argv[2];
const command = `printf '真实桌面执行日志\\n'; printf 'desktop-artifact\\n' > "$OURO_ARTIFACT_DIR/result.txt"`;
const mission = {
  name: '桌面真实任务',
  goal: '通过桌面 GUI smoke 启动真实任务',
  command,
};
const events = [];
let executionID = '';
let seenStarted = false;
let seenCompleted = false;

function remember(raw) {
  let event;
  try {
    event = JSON.parse(raw);
  } catch {
    return;
  }
  events.push(event);
  consumeEvents();
}

function consumeEvents() {
  if (!executionID) {
    return;
  }
  for (const event of events) {
    const payload = event.payload || {};
    if (payload.id !== executionID) {
      continue;
    }
    if (event.type === 'execution:started') {
      seenStarted = true;
    }
    if (event.type === 'execution:completed') {
      seenCompleted = true;
    }
  }
  if (seenStarted && seenCompleted) {
    clearTimeout(timeout);
    ws.close();
    process.stdout.write(executionID);
  }
}

const timeout = setTimeout(() => {
  console.error(`missing websocket execution events started=${seenStarted} completed=${seenCompleted} id=${executionID}`);
  process.exit(2);
}, 5000);

const ws = new WebSocket(`ws://127.0.0.1:${port}/api/events/stream`);
ws.addEventListener('message', event => remember(event.data));
ws.addEventListener('error', event => {
  clearTimeout(timeout);
  console.error(`websocket error: ${event.message || 'unknown'}`);
  process.exit(2);
});
ws.addEventListener('open', async () => {
  try {
    const response = await fetch(`http://127.0.0.1:${port}/api/mission/start`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(mission),
    });
    if (!response.ok) {
      throw new Error(`mission start POST returned ${response.status}`);
    }
    const created = await response.json();
    executionID = created.execution?.id || '';
    if (!executionID) {
      throw new Error(`execution id missing: ${JSON.stringify(created)}`);
    }
    consumeEvents();
  } catch (error) {
    clearTimeout(timeout);
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(2);
  }
});
NODE
)" || fail "execution websocket smoke failed"
[ -n "$execution_id" ] || fail "execution id not returned by websocket smoke"

curl -fsS "http://127.0.0.1:$API_PORT/api/executions/$execution_id/logs" > "$SMOKE_ROOT/execution-logs.json"
curl -fsS "http://127.0.0.1:$API_PORT/api/executions/$execution_id/artifacts" > "$SMOKE_ROOT/execution-artifacts.json"
curl -fsS "http://127.0.0.1:$API_PORT/api/gui/bootstrap" > "$SMOKE_ROOT/bootstrap-before-finding-links.json"
node - "$WORKSPACE/findings/high.json" "$SMOKE_ROOT/bootstrap-before-finding-links.json" "$execution_id" <<'NODE'
const fs = require('node:fs');
const [findingPath, bootstrapPath, executionID] = process.argv.slice(2);
const bootstrap = JSON.parse(fs.readFileSync(bootstrapPath, 'utf8'));
const event = (bootstrap.replayEvents || []).find(item => item.executionId === executionID);
if (!event?.id) {
  throw new Error(`replay event for ${executionID} not found`);
}
fs.writeFileSync(findingPath, JSON.stringify({
  title: '真实高危发现',
  severity: 'high',
  endpoint: 'https://desktop-smoke.local/admin',
  node_id: 'findings-high-json',
  file_path: 'findings/high.json',
  execution_id: executionID,
  replay_event_id: event.id,
}, null, 2));
NODE
curl -fsS "http://127.0.0.1:$API_PORT/api/gui/bootstrap" > "$SMOKE_ROOT/bootstrap.json"
assert_contains "$SMOKE_ROOT/execution-logs.json" "真实桌面执行日志"
assert_contains "$SMOKE_ROOT/execution-artifacts.json" "result.txt"
assert_contains "$SMOKE_ROOT/execution-artifacts.json" "desktop-artifact"
assert_contains "$WORKSPACE/mission.yaml" "桌面真实任务"
assert_contains "$WORKSPACE/mission.yaml" "通过桌面 GUI smoke 启动真实任务"
assert_contains "$SMOKE_ROOT/bootstrap.json" "replayEvents"
assert_contains "$SMOKE_ROOT/bootstrap.json" "reportFindings"
assert_contains "$SMOKE_ROOT/bootstrap.json" "真实高危发现"
assert_contains "$SMOKE_ROOT/bootstrap.json" "nodeId"
assert_contains "$SMOKE_ROOT/bootstrap.json" "filePath"
assert_contains "$SMOKE_ROOT/bootstrap.json" "executionId"
assert_contains "$SMOKE_ROOT/bootstrap.json" "replayEventId"
assert_contains "$SMOKE_ROOT/bootstrap.json" "findings-high-json"
assert_contains "$SMOKE_ROOT/bootstrap.json" "findings/high.json"
assert_contains "$SMOKE_ROOT/bootstrap.json" "通过桌面 GUI smoke 启动真实任务"
assert_contains "$SMOKE_ROOT/bootstrap.json" "真实桌面工作区"
assert_contains "$SMOKE_ROOT/bootstrap.json" "红队工程师"
assert_contains "$SMOKE_ROOT/bootstrap.json" "desktop-saved-model"
assert_contains "$SMOKE_ROOT/bootstrap.json" "OURO_DESKTOP_PROVIDER_KEY"
assert_contains "$SMOKE_ROOT/bootstrap.json" "local-pack-saved"
assert_contains "$SMOKE_ROOT/bootstrap.json" "jsonl_parser"
assert_contains "$SMOKE_ROOT/bootstrap.json" "runTimeoutMs"
assert_contains "$SMOKE_ROOT/bootstrap.json" "toolActions"
assert_contains "$SMOKE_ROOT/bootstrap.json" "工具动作日志"
assert_contains "$SMOKE_ROOT/bootstrap.json" "$tool_execution_id"
assert_contains "$SMOKE_ROOT/bootstrap.json" "$execution_id"

npm --prefix gui run test:contract >/dev/null
VITE_OURO_API_BASE_URL="http://127.0.0.1:$API_PORT" npm --prefix gui run build >/dev/null
npm --prefix gui run preview -- --port "$PREVIEW_PORT" > "$PREVIEW_LOG" 2>&1 &
PREVIEW_PID=$!
wait_http "http://127.0.0.1:$PREVIEW_PORT/" "frontend preview"

if command -v chromium >/dev/null 2>&1; then
  CHROMIUM=chromium
elif command -v chromium-browser >/dev/null 2>&1; then
  CHROMIUM=chromium-browser
else
  fail "chromium is required for desktop GUI smoke"
fi

"$CHROMIUM" --headless --disable-gpu --no-sandbox --window-size=1440,1000 --remote-debugging-port="$CDP_PORT" --user-data-dir="$SMOKE_ROOT/chromium-profile" "http://127.0.0.1:$PREVIEW_PORT/" > "$SMOKE_ROOT/chromium.log" 2>&1 &
CHROMIUM_PID=$!
wait_http "http://127.0.0.1:$CDP_PORT/json/version" "chromium devtools"

node - "$CDP_PORT" "http://127.0.0.1:$PREVIEW_PORT/" "$DOM_OUT" "http://127.0.0.1:$API_PORT" "$WORKSPACE" <<'NODE' || fail "chromium interaction smoke failed"
const [cdpPort, pageURL, outPath, apiBaseURL, workspacePath] = process.argv.slice(2);
const fs = await import('node:fs');

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

const pages = await fetch(`http://127.0.0.1:${cdpPort}/json/list`).then(response => response.json());
const page = pages.find(item => item.type === 'page') || pages[0];
if (!page?.webSocketDebuggerUrl) {
  throw new Error('chromium page websocket is unavailable');
}

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
  socket.addEventListener('error', () => reject(new Error('chromium websocket error')), { once: true });
});

function send(method, params = {}) {
  const id = nextID++;
  socket.send(JSON.stringify({ id, method, params }));
  return new Promise((resolve, reject) => pending.set(id, { resolve, reject }));
}

async function textContent() {
  const result = await send('Runtime.evaluate', {
    expression: 'document.body ? document.body.innerText : ""',
    returnByValue: true,
  });
  return result.result?.value || '';
}

async function waitForText(needle) {
  for (let i = 0; i < 80; i += 1) {
    const text = await textContent();
    if (text.includes(needle)) {
      return text;
    }
    await sleep(100);
  }
  throw new Error(`page text did not contain ${needle}`);
}

async function waitForReportSection(section, needle) {
  for (let i = 0; i < 80; i += 1) {
    const result = await send('Runtime.evaluate', {
      expression: `
        (() => {
          const section = document.querySelector('.report-section-content[data-report-section="${section}"]');
          const text = section ? section.innerText : "";
          return text.includes("${needle}") ? text : "";
        })()
      `,
      returnByValue: true,
    });
    if (result.result?.value) {
      return result.result.value;
    }
    await sleep(100);
  }
  throw new Error(`report section ${section} did not contain ${needle}`);
}

async function waitForSettingsSection(section, needle) {
  for (let i = 0; i < 80; i += 1) {
    const result = await send('Runtime.evaluate', {
      expression: `
        (() => {
          const section = document.querySelector('.settings-grid[data-settings-section="${section}"]');
          if (!section) return "";
          const values = Array.from(section.querySelectorAll('input, select, textarea'))
            .map(item => item.value || item.checked || "")
            .join("\\n");
          const text = section.innerText + "\\n" + values;
          return text.includes("${needle}") ? text : "";
        })()
      `,
      returnByValue: true,
    });
    if (result.result?.value) {
      return result.result.value;
    }
    await sleep(100);
  }
  throw new Error(`settings section ${section} did not contain ${needle}`);
}

async function waitForEvidenceFile(path) {
  for (let i = 0; i < 80; i += 1) {
    const text = await textContent();
    if (text.includes('文件编辑器') && text.includes(path) && text.includes('证据文件')) {
      return text;
    }
    await sleep(100);
  }
  throw new Error(`evidence file panel did not open: ${path}`);
}

async function waitForWorkspaceFile(path) {
  for (let i = 0; i < 80; i += 1) {
    const text = await textContent();
    if (text.includes('文件编辑器') && text.includes(path)) {
      return text;
    }
    await sleep(100);
  }
  throw new Error(`workspace file panel did not open: ${path}`);
}

async function waitForEditorContent(path, needle) {
  for (let i = 0; i < 80; i += 1) {
    const result = await send('Runtime.evaluate', {
      expression: `
        (() => {
          const text = document.body ? document.body.innerText : "";
          const editor = document.querySelector('.file-editor-panel textarea');
          const value = editor ? editor.value : "";
          return text.includes("${path}") && value.includes("${needle}");
        })()
      `,
      returnByValue: true,
    });
    if (result.result?.value) {
      return true;
    }
    await sleep(100);
  }
  throw new Error(`editor content for ${path} did not contain ${needle}`);
}

async function clickMode(label) {
  const result = await send('Runtime.evaluate', {
    expression: `
      (() => {
        const button = document.querySelector('button[aria-label="${label}"]');
        if (!button) return false;
        button.click();
        return true;
      })()
    `,
    returnByValue: true,
  });
  if (!result.result?.value) {
    throw new Error(`mode button not found: ${label}`);
  }
}

async function clickText(label) {
  const result = await send('Runtime.evaluate', {
    expression: `
      (() => {
        const buttons = Array.from(document.querySelectorAll('button'));
        const button = buttons.find(item => (item.textContent || '').includes("${label}"));
        if (!button) return false;
        button.click();
        return true;
      })()
    `,
    returnByValue: true,
  });
  if (!result.result?.value) {
    throw new Error(`button text not found: ${label}`);
  }
}

async function clickGraphNode(label) {
  const result = await send('Runtime.evaluate', {
    expression: `
      (() => {
        const buttons = Array.from(document.querySelectorAll('.graph-canvas .graph-node'));
        const button = buttons.find(item => (item.textContent || '').includes("${label}"));
        if (!button) return false;
        button.click();
        return true;
      })()
    `,
    returnByValue: true,
  });
  if (!result.result?.value) {
    throw new Error(`graph node text not found: ${label}`);
  }
}

await send('Page.enable');
await send('Runtime.enable');
await send('Page.addScriptToEvaluateOnNewDocument', {
  source: `
    window.ouroboros = {
      getAPIBaseURL: async () => ${JSON.stringify(apiBaseURL)},
      backendStatus: async () => ({ status: 'ready', apiBaseURL: ${JSON.stringify(apiBaseURL)}, workspace: ${JSON.stringify(workspacePath)}, running: true }),
      restartBackend: async () => ({ status: 'ready', apiBaseURL: ${JSON.stringify(apiBaseURL)}, workspace: ${JSON.stringify(workspacePath)}, running: true }),
      selectWorkspace: async () => ({ workspace: ${JSON.stringify(workspacePath)} }),
      openWorkspace: async path => ({ workspace: path }),
      createWorkspace: async () => ({ workspace: ${JSON.stringify(workspacePath)} }),
      recentWorkspaces: async () => [${JSON.stringify(workspacePath)}],
      openRecentWorkspace: async path => ({ workspace: path }),
      windowState: async () => ({ windowState: { width: 1440, height: 1000, maximized: false } }),
      minimizeWindow: async () => ({ minimized: true }),
      toggleMaximizeWindow: async () => ({ maximized: true }),
      closeWindow: async () => ({ closed: true }),
      onMenuAction: () => () => undefined,
      onBackendStatus: () => () => undefined,
    };
  `,
});
await send('Page.navigate', { url: pageURL });

const sections = [];
sections.push(await waitForText('任务启动'));
sections.push(await waitForText('执行详情'));
await clickText('命令面板');
sections.push(await waitForText('刷新真实数据'));
sections.push(await waitForText('打开最近工作区'));
sections.push(await waitForText('运行当前命令'));
await clickText('刷新真实数据');
sections.push(await waitForText('任务启动'));
await clickText('命令面板');
sections.push(await waitForText('打开最近工作区'));
await clickText('打开最近工作区');
sections.push(await waitForText('任务启动'));
await clickMode('设置');
sections.push(await waitForText('当前状态'));
sections.push(await waitForSettingsSection('workspace', '真实桌面工作区'));
await clickText('当前状态');
sections.push(await waitForSettingsSection('status', '/api/gui/bootstrap'));
await clickText('模型服务配置');
sections.push(await waitForSettingsSection('provider', 'desktop-saved-model'));
await clickText('工具链配置');
sections.push(await waitForSettingsSection('toolchain', 'local-pack-saved'));
await clickText('工作区配置');
sections.push(await waitForSettingsSection('workspace', '红队工程师'));
await clickMode('任务工作台');
sections.push(await waitForText('任务启动'));
await clickText('日志');
sections.push(await waitForText('打开日志文件'));
await clickText('产物');
sections.push(await waitForText('打开产物文件'));
await clickText('打开产物文件');
sections.push(await waitForWorkspaceFile('result.txt'));
sections.push(await waitForText('定位报告'));
await clickText('定位报告');
sections.push(await waitForText('发现详情'));
sections.push(await waitForText('关联执行文件'));
sections.push(await waitForText('载入执行日志'));
await clickText('载入执行日志');
sections.push(await waitForText('打开日志文件'));
sections.push(await waitForText('载入产物列表'));
await clickText('载入产物列表');
sections.push(await waitForText('打开产物文件'));
await clickText('打开产物文件');
sections.push(await waitForWorkspaceFile('result.txt'));
await clickMode('报告');
sections.push(await waitForText('发现详情'));
await clickText('风险评级');
sections.push(await waitForReportSection('risk', '风险分布'));
await clickText('项目资料');
sections.push(await waitForReportSection('project', '真实桌面工作区'));
await clickText('发现');
sections.push(await waitForReportSection('findings', '发现详情'));
await clickText('查看执行');
sections.push(await waitForText('执行详情'));
sections.push(await waitForText('定位回放'));
await clickText('定位回放');
sections.push(await waitForText('回放详情'));
sections.push(await waitForText('关联执行文件'));
sections.push(await waitForText('载入执行日志'));
await clickText('载入执行日志');
sections.push(await waitForText('打开日志文件'));
sections.push(await waitForText('载入产物列表'));
await clickText('载入产物列表');
sections.push(await waitForText('打开产物文件'));
await clickText('打开产物文件');
sections.push(await waitForWorkspaceFile('result.txt'));
await clickMode('回放');
sections.push(await waitForText('回放详情'));
sections.push(await waitForText('定位报告'));
await clickText('定位报告');
sections.push(await waitForText('发现详情'));
await clickMode('回放');
sections.push(await waitForText('回放详情'));
await clickText('查看执行');
sections.push(await waitForText('执行详情'));
sections.push(await waitForText('定位图谱'));
await clickText('定位图谱');
sections.push(await waitForText('节点详情'));
await clickMode('攻击图谱');
await clickGraphNode('findings/high.json');
sections.push(await waitForText('节点详情'));
sections.push(await waitForText('关联执行文件'));
sections.push(await waitForText('载入执行日志'));
await clickText('载入执行日志');
sections.push(await waitForText('打开日志文件'));
sections.push(await waitForText('载入产物列表'));
await clickText('载入产物列表');
sections.push(await waitForText('打开产物文件'));
await clickText('打开产物文件');
sections.push(await waitForWorkspaceFile('result.txt'));
await clickMode('攻击图谱');
sections.push(await waitForText('节点详情'));
sections.push(await waitForText('定位报告'));
await clickText('定位报告');
sections.push(await waitForText('发现详情'));
await clickMode('攻击图谱');
sections.push(await waitForText('节点详情'));
await clickText('导出图谱');
sections.push(await waitForWorkspaceFile('exports/attack-graph.json'));
await waitForEditorContent('exports/attack-graph.json', 'findings/high.json');
await clickMode('攻击图谱');
sections.push(await waitForText('节点详情'));
await clickMode('报告');
sections.push(await waitForText('发现详情'));
await clickText('导出报告');
sections.push(await waitForWorkspaceFile('reports/ouroboros-report.md'));
await waitForEditorContent('reports/ouroboros-report.md', '真实高危发现');
await clickMode('报告');
sections.push(await waitForText('发现详情'));
await clickText('附录');
sections.push(await waitForReportSection('appendix', '关联证据与执行'));
sections.push(await waitForReportSection('appendix', '已有报告文件'));
sections.push(await waitForReportSection('appendix', 'reports/ouroboros-report.md'));
await clickText('发现');
sections.push(await waitForReportSection('findings', '发现详情'));
await clickText('打开证据文件');
sections.push(await waitForEvidenceFile('findings/high.json'));
await clickMode('回放');
sections.push(await waitForText('回放详情'));

fs.writeFileSync(outPath, sections.join('\n\n--- MODE ---\n\n'));
socket.close();
NODE

assert_contains "$DOM_OUT" "工作区"
assert_contains "$DOM_OUT" "真实桌面工作区"
assert_contains "$DOM_OUT" "desktop-saved-model"
assert_contains "$DOM_OUT" "工具链"
assert_contains "$DOM_OUT" "当前状态"
assert_contains "$DOM_OUT" "工作区配置"
assert_contains "$DOM_OUT" "模型服务配置"
assert_contains "$DOM_OUT" "工具链配置"
assert_contains "$DOM_OUT" "任务启动"
assert_contains "$DOM_OUT" "工具启动"
assert_contains "$DOM_OUT" "运行工具"
assert_contains "$DOM_OUT" "刷新真实数据"
assert_contains "$DOM_OUT" "打开最近工作区"
assert_contains "$DOM_OUT" "运行当前命令"
assert_contains "$DOM_OUT" "保存当前任务"
assert_contains "$DOM_OUT" "导出图谱"
assert_contains "$DOM_OUT" "exports/attack-graph.json"
assert_contains "$DOM_OUT" "导出报告"
assert_contains "$DOM_OUT" "reports/ouroboros-report.md"
assert_contains "$DOM_OUT" "风险分布"
assert_contains "$DOM_OUT" "项目资料"
assert_contains "$DOM_OUT" "关联证据与执行"
assert_contains "$DOM_OUT" "已有报告文件"
assert_contains "$DOM_OUT" "打开日志文件"
assert_contains "$DOM_OUT" "打开产物文件"
assert_contains "$DOM_OUT" "载入执行日志"
assert_contains "$DOM_OUT" "载入产物列表"
assert_contains "$DOM_OUT" "local-pack-saved"
assert_contains "$DOM_OUT" "工具动作日志"
assert_contains "$DOM_OUT" "任务目标"
assert_contains "$DOM_OUT" "运行命令"
assert_contains "$DOM_OUT" "通过桌面 GUI smoke 启动真实任务"
assert_contains "$DOM_OUT" "真实桌面执行日志"
assert_contains "$DOM_OUT" "产物"
assert_contains "$DOM_OUT" "定位图谱"
assert_contains "$DOM_OUT" "打开证据文件"
assert_contains "$DOM_OUT" "查看执行"
assert_contains "$DOM_OUT" "定位回放"
assert_contains "$DOM_OUT" "定位报告"
assert_contains "$DOM_OUT" "节点详情"
assert_contains "$DOM_OUT" "回放详情"
assert_contains "$DOM_OUT" "发现详情"
assert_contains "$DOM_OUT" "证据文件"
assert_contains "$DOM_OUT" "执行详情"
assert_contains "$DOM_OUT" "原始字段"
assert_contains "$DOM_OUT" "关联对象"
assert_contains "$DOM_OUT" "关联执行文件"

npm --prefix gui run build >/dev/null
printf 'desktop-gui-smoke: ok\n'
printf 'desktop-gui-smoke: root %s\n' "$SMOKE_ROOT"
