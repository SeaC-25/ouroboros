import { app, BrowserWindow, ipcMain, Menu, nativeImage, shell } from 'electron';
import { fileURLToPath } from 'node:url';
import fs from 'node:fs/promises';
import path from 'node:path';
import { BackendManager } from './backend.js';
import { DesktopStateManager } from './desktop-state.js';
import { WorkspaceManager } from './workspace.js';
import { loadOuroEnv } from './env.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const isDev = !app.isPackaged;
const backend = new BackendManager({ app });
const desktopState = new DesktopStateManager({ app });
const workspace = new WorkspaceManager({ app });
let mainWindow = null;
let backendStopBeforeQuit = false;
let fatalShutdownStarted = false;
const singleInstanceLock = app.requestSingleInstanceLock();

if (!singleInstanceLock) {
  app.quit();
} else {
  // .env is convenience configuration only (backend URL/keys via OURO_*);
  // load it before anything reads process.env and never let it block startup.
  try {
    const loaded = loadOuroEnv({ app });
    if (loaded.length > 0) {
      console.log(`已加载环境配置文件: ${loaded.join(', ')}`);
    }
  } catch (error) {
    console.error('加载环境配置文件失败:', error);
  }
  configureElectronRuntime();
  registerProcessDiagnostics();
  backend.startDesktopRuntimeSession();

  app.whenReady().then(async () => {
    registerIPC();
    backend.on('status', status => {
      broadcastBackendStatus(status);
    });
    await createWindow();

    app.on('activate', () => {
      if (BrowserWindow.getAllWindows().length === 0) {
        void createWindow();
      }
    });
  }).catch(error => {
    void shutdownFromFatalError('desktop.ready_failed', error);
  });

  app.on('second-instance', (_event, argv, workingDirectory) => {
    const status = backend.snapshot();
    backend.writeDiagnostic('desktop.second_instance', {
      argv,
      workingDirectory,
      status: status.status,
      processId: status.processId,
      workspace: status.workspace,
      apiBaseURL: status.apiBaseURL,
    });
    focusMainWindow();
    sendBackendSnapshotToWindow(mainWindow, 'second-instance');
    sendMenuAction(mainWindow, 'refresh');
  });
}

function configureElectronRuntime() {
  app.disableHardwareAcceleration();
  app.commandLine.appendSwitch('disable-gpu');
  app.commandLine.appendSwitch('disable-gpu-compositing');
  app.commandLine.appendSwitch('disable-gpu-sandbox');
  app.commandLine.appendSwitch('disable-vulkan');
  app.commandLine.appendSwitch('disable-webgl');
  app.commandLine.appendSwitch('disable-3d-apis');
  app.commandLine.appendSwitch('disable-features', 'Vulkan,WebGPU,UseSkiaRenderer,CanvasOopRasterization,VizDisplayCompositor');
}

async function createWindow() {
  const windowState = await desktopState.loadWindowState();
  const windowIcon = nativeImage.createFromPath(path.join(__dirname, 'assets', 'ouroboros-icon.png'));
  if (windowIcon.isEmpty()) {
    throw new Error('无法加载 Ouroboros 桌面蛇图标');
  }
  const window = new BrowserWindow({
    x: windowState.x,
    y: windowState.y,
    width: windowState.width,
    height: windowState.height,
    minWidth: 960,
    minHeight: 640,
    backgroundColor: '#0a0a0a',
    icon: windowIcon,
    title: 'Ouroboros',
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'default',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false
    }
  });
  if (process.platform !== 'darwin') {
    window.setIcon(windowIcon);
  }
  mainWindow = window;
  wireDesktopProcessDiagnostics(window);
  wireWindowNavigationGuards(window);
  if (windowState.maximized) {
    window.maximize();
  }
  wireWindowStatePersistence(window);
  wireRendererStartupSync(window);

  const lastWorkspace = await workspace.loadLastWorkspace();
  applyApplicationMenu(window);
  if (lastWorkspace) {
    void backend.start(lastWorkspace).then(() => {
      applyApplicationMenu(window);
      sendBackendSnapshotToWindow(window, 'workspace-auto-start');
      sendMenuAction(window, 'refresh');
    }).catch(error => {
      backend.writeDiagnostic('desktop.auto_start_failed', diagnosticError(error));
      sendBackendSnapshotToWindow(window, 'workspace-auto-start-failed');
      console.error('启动后端失败:', error);
    });
  }

  if (isDev) {
    await window.loadURL(developmentServerURL());
    window.webContents.openDevTools({ mode: 'detach' });
    return;
  }

  await window.loadFile(path.join(__dirname, '../renderer/index.html'));
}

function developmentServerURL() {
  const value = String(process.env.VITE_DEV_SERVER_URL || '').trim();
  if (!value) {
    throw new Error('桌面开发启动器未提供 Vite 服务地址');
  }
  let url;
  try {
    url = new URL(value);
  } catch {
    throw new Error(`桌面开发 Vite 服务地址无效：${value}`);
  }
  if (!['http:', 'https:'].includes(url.protocol)) {
    throw new Error(`桌面开发 Vite 服务协议无效：${url.protocol}`);
  }
  return url.href;
}

function registerIPC() {
  ipcMain.handle('backend:get-api-base-url', () => backend.snapshot().apiBaseURL || '');
  ipcMain.handle('backend:status', () => backend.snapshot());
  ipcMain.handle('backend:restart', async () => {
    const status = await backend.restart();
    sendMenuAction(mainWindow, 'refresh');
    return status;
  });
  ipcMain.handle('workspace:select', async event => {
    const parent = BrowserWindow.fromWebContents(event.sender);
    return openWorkspaceFromDialog(parent ?? undefined, 'select');
  });
  ipcMain.handle('workspace:open', async (_event, workspacePath) => {
    return openWorkspacePath(workspacePath, 'workspace:opened');
  });
  ipcMain.handle('workspace:create', async event => {
    const parent = BrowserWindow.fromWebContents(event.sender);
    return createWorkspaceFromDialog(parent ?? undefined);
  });
  ipcMain.handle('desktop:recent-workspaces', () => workspace.recentWorkspaces());
  ipcMain.handle('desktop:open-recent-workspace', async (_event, workspacePath) => openWorkspacePath(workspacePath, 'workspace:opened'));
  ipcMain.handle('desktop:show-workspace-file', async (_event, filePath) => showWorkspaceFile(filePath));
  ipcMain.handle('desktop:window-state', () => desktopState.snapshot());
  ipcMain.handle('desktop:runtime-info', () => backend.runtimeInfo());
  ipcMain.handle('window:minimize', event => {
    const window = BrowserWindow.fromWebContents(event.sender);
    if (!window || window.isDestroyed()) {
      return { minimized: false };
    }
    window.minimize();
    return { minimized: true };
  });
  ipcMain.handle('window:toggle-maximize', event => {
    const window = BrowserWindow.fromWebContents(event.sender);
    if (!window || window.isDestroyed()) {
      return { maximized: false };
    }
    if (window.isMaximized()) {
      window.unmaximize();
    } else {
      window.maximize();
    }
    return { maximized: window.isMaximized() };
  });
  ipcMain.handle('window:close', event => {
    const window = BrowserWindow.fromWebContents(event.sender);
    if (!window || window.isDestroyed()) {
      return { closed: false };
    }
    window.close();
    return { closed: true };
  });
}

function buildApplicationMenu(window) {
  const recent = workspace.recentWorkspaces();
  const recentSubmenu = recent.length > 0
    ? recent.map(workspacePath => ({
      label: path.basename(workspacePath) || workspacePath,
      sublabel: workspacePath,
      click: () => {
        void openWorkspacePath(workspacePath, 'workspace:opened');
      },
    }))
    : [{ label: '暂无最近工作区', enabled: false }];

  return Menu.buildFromTemplate([
    {
      label: '文件',
      submenu: [
        {
          label: '打开工作区',
          accelerator: 'CmdOrCtrl+O',
          click: () => {
            void openWorkspaceFromDialog(window, 'select');
          },
        },
        {
          label: '新建工作区',
          accelerator: 'CmdOrCtrl+Shift+O',
          click: () => {
            void createWorkspaceFromDialog(window);
          },
        },
        { label: '最近工作区', submenu: recentSubmenu },
        { type: 'separator' },
        { role: process.platform === 'darwin' ? 'close' : 'quit', label: process.platform === 'darwin' ? '关闭窗口' : '退出' },
      ],
    },
    {
      label: '视图',
      submenu: [
        menuModeItem(window, 'mission', '任务工作台', 'CmdOrCtrl+1'),
        menuModeItem(window, 'graph', '攻击图谱', 'CmdOrCtrl+2'),
        menuModeItem(window, 'replay', '回放', 'CmdOrCtrl+3'),
        menuModeItem(window, 'report', '报告', 'CmdOrCtrl+4'),
        menuModeItem(window, 'runtime', '运行时', 'CmdOrCtrl+5'),
        menuModeItem(window, 'settings', '设置', 'CmdOrCtrl+,'),
        { type: 'separator' },
        {
          label: '刷新真实数据',
          accelerator: 'CmdOrCtrl+R',
          click: () => sendMenuAction(window, 'refresh'),
        },
      ],
    },
    {
      label: '执行',
      submenu: [
        {
          label: '打开执行面板',
          accelerator: 'CmdOrCtrl+E',
          click: () => sendMenuAction(window, 'mode:mission'),
        },
        {
          label: '重启后端',
          click: () => {
            void backend.restart().then(() => {
              sendBackendSnapshotToWindow(window, 'menu-backend-restart');
              sendMenuAction(window, 'refresh');
            }).catch(error => {
              backend.writeDiagnostic('desktop.menu_restart_failed', diagnosticError(error));
              sendBackendSnapshotToWindow(window, 'menu-backend-restart-failed');
            });
          },
        },
      ],
    },
  ]);
}

function menuModeItem(window, mode, label, accelerator) {
  return {
    label,
    accelerator,
    click: () => sendMenuAction(window, `mode:${mode}`),
  };
}

function applyApplicationMenu(window) {
  Menu.setApplicationMenu(buildApplicationMenu(window));
}

async function openWorkspaceFromDialog(parent, mode) {
  const selected = mode === 'select'
    ? await workspace.selectWorkspace(parent)
    : '';
  if (selected) {
    await backend.start(selected);
    applyApplicationMenu(mainWindow);
    sendMenuAction(mainWindow, 'refresh');
  }
  return { ...workspace.snapshot(), backend: backend.snapshot() };
}

async function createWorkspaceFromDialog(parent) {
  const selected = await workspace.createWorkspace(parent);
  if (selected) {
    await backend.initializeWorkspace(selected);
    await backend.start(selected);
    applyApplicationMenu(mainWindow);
    sendMenuAction(mainWindow, 'refresh');
  }
  return { ...workspace.snapshot(), backend: backend.snapshot() };
}

async function openWorkspacePath(workspacePath, action) {
  const selected = await workspace.openWorkspace(workspacePath);
  if (selected) {
    await backend.start(selected);
    applyApplicationMenu(mainWindow);
    sendMenuAction(mainWindow, action);
    sendMenuAction(mainWindow, 'refresh');
  }
  return { ...workspace.snapshot(), backend: backend.snapshot() };
}

async function showWorkspaceFile(filePath) {
  const workspacePath = workspace.snapshot().workspace;
  if (!workspacePath || typeof filePath !== 'string' || filePath.trim() === '') {
    return { shown: false, path: '', reason: 'missing-workspace-file' };
  }
  const root = path.resolve(workspacePath);
  const absolutePath = path.resolve(root, filePath);
  const relativePath = path.relative(root, absolutePath);
  if (relativePath === '' || relativePath.startsWith('..') || path.isAbsolute(relativePath)) {
    return { shown: false, path: '', reason: 'outside-workspace' };
  }
  try {
    const stat = await fs.stat(absolutePath);
    if (!stat.isFile()) {
      return { shown: false, path: absolutePath, reason: 'not-file' };
    }
  } catch {
    return { shown: false, path: absolutePath, reason: 'missing-file' };
  }
  shell.showItemInFolder(absolutePath);
  return { shown: true, path: absolutePath };
}

function sendMenuAction(window, action) {
  if (!window || window.isDestroyed()) {
    return;
  }
  try {
    window.webContents.send('desktop:menu-action', action);
  } catch (error) {
    backend.writeDiagnostic('desktop.menu_action_send_failed', diagnosticError(error));
  }
}

function broadcastBackendStatus(status) {
  for (const window of BrowserWindow.getAllWindows()) {
    if (!window.isDestroyed()) {
      try {
        window.webContents.send('backend:status-changed', status);
      } catch (error) {
        backend.writeDiagnostic('desktop.backend_status_broadcast_failed', diagnosticError(error));
      }
    }
  }
}

function sendBackendSnapshotToWindow(window, reason) {
  if (!window || window.isDestroyed()) {
    return;
  }
  const status = backend.snapshot();
  backend.writeDiagnostic('desktop.renderer_status_sync', {
    reason,
    status: status.status,
    processId: status.processId,
    workspace: status.workspace,
    apiBaseURL: status.apiBaseURL,
  });
  try {
    window.webContents.send('backend:status-changed', status);
  } catch (error) {
    backend.writeDiagnostic('desktop.renderer_status_send_failed', diagnosticError(error));
  }
}

function focusMainWindow() {
  if (!mainWindow || mainWindow.isDestroyed()) {
    return;
  }
  if (mainWindow.isMinimized()) {
    mainWindow.restore();
  }
  mainWindow.show();
  mainWindow.focus();
}

function wireWindowStatePersistence(window) {
  let saveTimer = null;
  const scheduleSave = () => {
    if (saveTimer) {
      clearTimeout(saveTimer);
    }
    saveTimer = setTimeout(() => {
      void desktopState.saveWindowState(window).then(() => sendMenuAction(window, 'window-state:saved'));
    }, 300);
  };
  window.on('resize', scheduleSave);
  window.on('move', scheduleSave);
  window.on('close', () => {
    if (saveTimer) {
      clearTimeout(saveTimer);
    }
    void desktopState.saveWindowState(window);
  });
  window.on('closed', () => {
    if (mainWindow === window) {
      mainWindow = null;
    }
  });
}

function wireRendererStartupSync(window) {
  window.webContents.once('did-finish-load', () => {
    sendBackendSnapshotToWindow(window, 'did-finish-load');
    sendMenuAction(window, 'refresh');
  });
}

// Allow only the two origins this desktop app actually uses: the packaged
// file:// renderer and the Vite dev server. Anything else is a renderer
// compromise attempt (e.g. injected Markdown links): new windows are denied
// outright, same-window navigation is blocked, and external http(s) URLs are
// handed to the system browser instead.
function wireWindowNavigationGuards(window) {
  const devServerOrigin = (() => {
    const value = String(process.env.VITE_DEV_SERVER_URL || '').trim();
    if (!value) {
      return '';
    }
    try {
      return new URL(value).origin;
    } catch {
      return '';
    }
  })();

  const isAllowedNavigationURL = url => {
    if (!url || typeof url !== 'string') {
      return false;
    }
    if (url.startsWith('file:')) {
      // Only the packaged renderer document; URLs are percent-encoded, so the
      // drive-letter form on Windows ('/F:/...') is covered by the /F:/
      // branch. devtools or other file paths are rejected.
      return /^file:\/\/\/[A-Za-z]:\/.*\/renderer\/index\.html$/i.test(url);
    }
    try {
      const parsed = new URL(url);
      return Boolean(devServerOrigin) && parsed.origin === devServerOrigin;
    } catch {
      return false;
    }
  };

  window.webContents.setWindowOpenHandler(details => {
    const url = details.url || '';
    if (/^https?:/i.test(url)) {
      void shell.openExternal(url).catch(error => {
        backend.writeDiagnostic('desktop.open_external_failed', diagnosticError(error));
      });
    }
    backend.writeDiagnostic('desktop.window_open_denied', {
      url: String(url).slice(-2000),
      disposition: details.disposition || '',
    });
    return { action: 'deny' };
  });

  window.webContents.on('will-navigate', (event, url) => {
    if (isAllowedNavigationURL(url)) {
      return;
    }
    event.preventDefault();
    if (/^https?:/i.test(url)) {
      void shell.openExternal(url).catch(error => {
        backend.writeDiagnostic('desktop.open_external_failed', diagnosticError(error));
      });
    }
    backend.writeDiagnostic('desktop.will_navigate_blocked', {
      url: String(url).slice(-2000),
    });
  });
}

function wireDesktopProcessDiagnostics(window) {
  window.on('unresponsive', () => {
    recordDesktopRuntimeIssue('desktop.window_unresponsive', window, {
      message: '窗口无响应',
    });
  });
  window.on('responsive', () => {
    backend.writeDiagnostic('desktop.window_responsive', {
      windowId: window.id,
    });
  });
  window.webContents.on('render-process-gone', (_event, details) => {
    recordDesktopRuntimeIssue('desktop.render_process_gone', window, details);
    sendBackendSnapshotToWindow(window, 'render-process-gone');
  });
  window.webContents.on('did-fail-load', (_event, errorCode, errorDescription, validatedURL, isMainFrame) => {
    recordDesktopRuntimeIssue('desktop.renderer_load_failed', window, {
      errorCode,
      errorDescription,
      validatedURL,
      isMainFrame,
    });
  });
}

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

app.on('before-quit', event => {
  const status = backend.snapshot();
  if (backendStopBeforeQuit || !status.running) {
    return;
  }
  event.preventDefault();
  backendStopBeforeQuit = true;
  void stopBackendBeforeQuit(status, 'before-quit').finally(() => app.quit());
});

function registerProcessDiagnostics() {
  app.on('child-process-gone', (_event, details) => {
    backend.recordDesktopRuntimeIssue('desktop.child_process_gone', normalizeChildProcessGoneDetails(details));
    sendBackendSnapshotToWindow(mainWindow, 'child-process-gone');
  });
  app.on('will-quit', () => {
    backend.stopDesktopRuntimeSession('will-quit');
  });
  app.on('quit', () => {
    backend.stopDesktopRuntimeSession('quit');
  });
  process.on('uncaughtException', error => {
    void shutdownFromFatalError('desktop.uncaught_exception', error);
  });
  process.on('unhandledRejection', reason => {
    void shutdownFromFatalError('desktop.unhandled_rejection', reason);
  });
  for (const signal of ['SIGINT', 'SIGTERM']) {
    process.on(signal, () => {
      void shutdownFromProcessSignal(signal);
    });
  }
}

async function shutdownFromFatalError(event, error) {
  if (fatalShutdownStarted) {
    return;
  }
  fatalShutdownStarted = true;
  backendStopBeforeQuit = true;
  const details = diagnosticError(error);
  backend.recordDesktopRuntimeIssue(event, details);
  backend.stopDesktopRuntimeSession(`fatal:${event}`, 'crashed');
  console.error('桌面端发生致命错误:', error);
  await stopBackendBeforeQuit(backend.snapshot(), event);
  backend.stopDesktopRuntimeSession(`fatal:${event}`, 'crashed');
  app.exit(1);
}

async function shutdownFromProcessSignal(signal) {
  if (backendStopBeforeQuit) {
    return;
  }
  backendStopBeforeQuit = true;
  const status = backend.snapshot();
  backend.writeDiagnostic('desktop.process_signal', {
    signal,
    status: status.status,
    processId: status.processId,
    workspace: status.workspace,
    apiBaseURL: status.apiBaseURL,
  });
  await stopBackendBeforeQuit(status, signal);
  backend.stopDesktopRuntimeSession(`process-signal:${signal}`);
  app.exit(0);
}

async function stopBackendBeforeQuit(status, reason) {
  try {
    backend.writeDiagnostic('backend.before_quit', {
      reason,
      status: status.status,
      processId: status.processId,
      workspace: status.workspace,
      apiBaseURL: status.apiBaseURL,
    });
    await backend.stop();
  } catch (error) {
    console.error('停止后端失败:', error);
  }
}

function diagnosticError(error) {
  if (error instanceof Error) {
    return {
      message: error.message,
      stack: error.stack || '',
    };
  }
  return {
    message: String(error),
    stack: '',
  };
}

function recordDesktopRuntimeIssue(event, window, details = {}) {
  const status = backend.snapshot();
  backend.recordDesktopRuntimeIssue(event, {
    ...normalizeChildProcessGoneDetails(details),
    windowId: window?.id ?? 0,
    status: status.status,
    processId: status.processId,
    workspace: status.workspace,
    apiBaseURL: status.apiBaseURL,
  });
}

function normalizeChildProcessGoneDetails(details = {}) {
  return {
    type: typeof details.type === 'string' ? details.type : '',
    reason: typeof details.reason === 'string' ? details.reason : '',
    exitCode: typeof details.exitCode === 'number' ? details.exitCode : null,
    serviceName: typeof details.serviceName === 'string' ? details.serviceName : '',
    name: typeof details.name === 'string' ? details.name : '',
    message: typeof details.message === 'string' ? details.message : '',
    errorCode: typeof details.errorCode === 'number' ? details.errorCode : null,
    errorDescription: typeof details.errorDescription === 'string' ? details.errorDescription : '',
    validatedURL: typeof details.validatedURL === 'string' ? details.validatedURL : '',
    isMainFrame: typeof details.isMainFrame === 'boolean' ? details.isMainFrame : null,
  };
}
