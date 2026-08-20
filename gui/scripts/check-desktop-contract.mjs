import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('..', import.meta.url));
const repoRoot = fileURLToPath(new URL('../..', import.meta.url));
let failed = false;

for (const file of ['src/main/main.js', 'src/main/env.js', 'src/main/preload.js', 'src/main/backend.js', 'src/main/workspace.js', 'src/main/desktop-state.js', 'scripts/package-desktop.mjs']) {
  if (!existsSync(join(root, file))) {
    console.error(`missing desktop file: ${file}`);
    failed = true;
  }
}

const main = read('src/main/main.js');
for (const required of ['Menu', 'buildApplicationMenu', 'preload:', 'backend:get-api-base-url', 'backend:status', 'backend:restart', 'workspace:select', 'workspace:open', 'workspace:create', 'desktop:recent-workspaces', 'desktop:open-recent-workspace', 'desktop:window-state', 'window:minimize', 'window:toggle-maximize', 'window:close', 'desktop:menu-action', 'initializeWorkspace', 'BackendManager', 'WorkspaceManager', 'DesktopStateManager', 'loadOuroEnv']) {
  if (!main.includes(required)) {
    console.error(`main process missing ${required}`);
    failed = true;
  }
}

const preload = read('src/main/preload.js');
for (const required of ['contextBridge.exposeInMainWorld', 'getAPIBaseURL', 'backendStatus', 'restartBackend', 'selectWorkspace', 'openWorkspace', 'createWorkspace', 'recentWorkspaces', 'openRecentWorkspace', 'windowState', 'minimizeWindow', 'toggleMaximizeWindow', 'closeWindow', 'onMenuAction', 'onBackendStatus']) {
  if (!preload.includes(required)) {
    console.error(`preload bridge missing ${required}`);
    failed = true;
  }
}
if (preload.includes("import { contextBridge, ipcRenderer } from 'electron'") || preload.includes('import { contextBridge, ipcRenderer } from "electron"')) {
  console.error('preload bridge must be packaged-runtime compatible CommonJS, not a top-level ESM import');
  failed = true;
}
if (!preload.includes("require('electron')") && !preload.includes('require("electron")')) {
  console.error('preload bridge must require electron so packaged Electron can load the preload script');
  failed = true;
}

const shell = read('src/app/OuroborosIDE.tsx');
for (const required of ['minimizeDesktopWindow', 'toggleMaximizeDesktopWindow', 'closeDesktopWindow', 'zhCN.windowControls.minimize', 'zhCN.windowControls.maximize', 'zhCN.windowControls.close', 'onMinimizeWindow', 'onToggleMaximizeWindow', 'onCloseWindow']) {
  if (!shell.includes(required)) {
    console.error(`renderer missing real window control: ${required}`);
    failed = true;
  }
}
if (shell.includes('<span />\n        <span />\n        <span />')) {
  console.error('renderer window controls must not be static spans');
  failed = true;
}

const backend = read('src/main/backend.js');
for (const required of ['EventEmitter', 'spawn(', 'go', 'serve', 'init', '/api/health', 'choosePort', 'initializeWorkspace', 'restart()', "status = 'crashed'", 'lastError', 'emitStatus']) {
  if (!backend.includes(required)) {
    console.error(`backend manager missing ${required}`);
    failed = true;
  }
}

const workspace = read('src/main/workspace.js');
for (const required of ['recentWorkspaces', 'rememberWorkspace', 'createWorkspace', '选择或创建 Ouroboros 工作区']) {
  if (!workspace.includes(required)) {
    console.error(`workspace manager missing ${required}`);
    failed = true;
  }
}
const createWorkspaceBody = workspace.match(/async createWorkspace\(parentWindow\) \{([\s\S]*?)\n  async openWorkspace/);
if (!createWorkspaceBody || !/if \(result\.canceled \|\| result\.filePaths\.length === 0\) \{\s*return '';\s*\}/m.test(createWorkspaceBody[1])) {
  console.error('createWorkspace must return an empty selection when the dialog is canceled');
  failed = true;
}

const desktopState = read('src/main/desktop-state.js');
for (const required of ['DesktopStateManager', 'loadWindowState', 'saveWindowState', 'window-state.json']) {
  if (!desktopState.includes(required)) {
    console.error(`desktop state manager missing ${required}`);
    failed = true;
  }
}

const packager = read('scripts/package-desktop.mjs');
for (const required of ['build.sh', 'dist', 'resources', '--dry-run', '--dir', 'copyElectronRuntime', 'resources/app', 'package:desktop:dir']) {
  if (!packager.includes(required)) {
    console.error(`desktop package preparation missing ${required}`);
    failed = true;
  }
}

const packageJSON = read('package.json');
for (const required of ['package:desktop:dir', 'package:desktop:dir:dry-run']) {
  if (!packageJSON.includes(required)) {
    console.error(`package.json missing desktop package script: ${required}`);
    failed = true;
  }
}

const packageSmoke = readRepo('scripts/smoke_desktop_package.sh');
for (const required of ['sidecar/layout', 'BACKEND_BINARY', 'packaged backend sidecar']) {
  if (!packageSmoke.includes(required)) {
    console.error(`desktop package sidecar/layout smoke must identify itself clearly: ${required}`);
    failed = true;
  }
}

const packageRuntimeSmoke = readRepo('scripts/smoke_desktop_package_runtime.sh');
for (const required of ['APP_BINARY', '--remote-debugging-port', 'window.ouroboros', 'getAPIBaseURL', 'backendStatus', 'OUROBOROS_WORKSPACE', 'XDG_CONFIG_HOME', 'xvfb-run']) {
  if (!packageRuntimeSmoke.includes(required)) {
    console.error(`desktop package runtime smoke missing real Electron coverage: ${required}`);
    failed = true;
  }
}
if (packageRuntimeSmoke.includes('window.ouroboros =') || packageRuntimeSmoke.includes('Object.defineProperty(window, \'ouroboros\'') || packageRuntimeSmoke.includes('Object.defineProperty(window, "ouroboros"')) {
  console.error('desktop package runtime smoke must use the real preload bridge, not inject window.ouroboros');
  failed = true;
}

for (const required of ['smoke:desktop-package:runtime', 'smoke_desktop_package_runtime.sh']) {
  if (!packageJSON.includes(required)) {
    console.error(`package.json missing desktop runtime smoke script: ${required}`);
    failed = true;
  }
}

if (failed) {
  process.exit(1);
}

function read(file) {
  const fullPath = join(root, file);
  return existsSync(fullPath) ? readFileSync(fullPath, 'utf8') : '';
}

function readRepo(file) {
  const fullPath = join(repoRoot, file);
  return existsSync(fullPath) ? readFileSync(fullPath, 'utf8') : '';
}
