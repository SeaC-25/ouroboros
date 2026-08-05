import { app, BrowserWindow, ipcMain, Menu } from 'electron';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { BackendManager } from './backend.js';
import { DesktopStateManager } from './desktop-state.js';
import { WorkspaceManager } from './workspace.js';
import { loadOuroEnv } from './env.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

loadOuroEnv({ app });

const isDev = !app.isPackaged;
const backend = new BackendManager({ app });
const desktopState = new DesktopStateManager({ app });
const workspace = new WorkspaceManager({ app });
let mainWindow = null;

async function createWindow() {
  const windowState = await desktopState.loadWindowState();
  const window = new BrowserWindow({
    x: windowState.x,
    y: windowState.y,
    width: windowState.width,
    height: windowState.height,
    minWidth: 960,
    minHeight: 640,
    backgroundColor: '#0a0a0a',
    title: 'Ouroboros',
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'default',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false
    }
  });
  mainWindow = window;
  if (windowState.maximized) {
    window.maximize();
  }
  wireWindowStatePersistence(window);

  const lastWorkspace = await workspace.loadLastWorkspace();
  applyApplicationMenu(window);
  if (lastWorkspace) {
    void backend.start(lastWorkspace).catch(error => {
      console.error('启动后端失败:', error);
    });
  }

  if (isDev) {
    window.loadURL(process.env.VITE_DEV_SERVER_URL ?? 'http://127.0.0.1:5173');
    window.webContents.openDevTools({ mode: 'detach' });
    return;
  }

  window.loadFile(path.join(__dirname, '../renderer/index.html'));
}

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
});

function registerIPC() {
  ipcMain.handle('backend:get-api-base-url', () => backend.snapshot().apiBaseURL || process.env.VITE_OURO_API_BASE_URL || '');
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
  ipcMain.handle('desktop:window-state', () => desktopState.snapshot());
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
            sendMenuAction(window, 'backend:restart');
            void backend.restart().then(() => sendMenuAction(window, 'refresh'));
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

function sendMenuAction(window, action) {
  if (!window || window.isDestroyed()) {
    return;
  }
  window.webContents.send('desktop:menu-action', action);
}

function broadcastBackendStatus(status) {
  for (const window of BrowserWindow.getAllWindows()) {
    if (!window.isDestroyed()) {
      window.webContents.send('backend:status-changed', status);
    }
  }
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

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

app.on('before-quit', () => {
  void backend.stop();
});
