const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('ouroboros', {
  getAPIBaseURL: () => ipcRenderer.invoke('backend:get-api-base-url'),
  backendStatus: () => ipcRenderer.invoke('backend:status'),
  restartBackend: () => ipcRenderer.invoke('backend:restart'),
  selectWorkspace: () => ipcRenderer.invoke('workspace:select'),
  openWorkspace: path => ipcRenderer.invoke('workspace:open', path),
  createWorkspace: () => ipcRenderer.invoke('workspace:create'),
  recentWorkspaces: () => ipcRenderer.invoke('desktop:recent-workspaces'),
  openRecentWorkspace: path => ipcRenderer.invoke('desktop:open-recent-workspace', path),
  showWorkspaceFile: path => ipcRenderer.invoke('desktop:show-workspace-file', path),
  windowState: () => ipcRenderer.invoke('desktop:window-state'),
  runtimeInfo: () => ipcRenderer.invoke('desktop:runtime-info'),
  minimizeWindow: () => ipcRenderer.invoke('window:minimize'),
  toggleMaximizeWindow: () => ipcRenderer.invoke('window:toggle-maximize'),
  closeWindow: () => ipcRenderer.invoke('window:close'),
  onMenuAction: callback => {
    const listener = (_event, action) => callback(action);
    ipcRenderer.on('desktop:menu-action', listener);
    return () => ipcRenderer.removeListener('desktop:menu-action', listener);
  },
  onBackendStatus: callback => {
    const listener = (_event, status) => callback(status);
    ipcRenderer.on('backend:status-changed', listener);
    return () => ipcRenderer.removeListener('backend:status-changed', listener);
  },
});
