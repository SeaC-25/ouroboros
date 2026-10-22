import { dialog } from 'electron';
import fs from 'node:fs/promises';
import path from 'node:path';

export class WorkspaceManager {
  constructor({ app }) {
    this.app = app;
    this.statePath = path.join(app.getPath('userData'), 'workspace.json');
    this.current = process.env.OUROBOROS_WORKSPACE || '';
    this.recent = [];
  }

  async loadLastWorkspace() {
    if (this.current) {
      await this.rememberWorkspace(this.current);
      return this.current;
    }
    try {
      const state = JSON.parse(await fs.readFile(this.statePath, 'utf8'));
      this.current = typeof state.lastWorkspace === 'string' ? state.lastWorkspace : '';
      this.recent = Array.isArray(state.recentWorkspaces)
        ? state.recentWorkspaces.filter(item => typeof item === 'string' && item.trim() !== '').slice(0, 10)
        : [];
    } catch {
      this.current = '';
      this.recent = [];
    }
    return this.current;
  }

  async selectWorkspace(parentWindow) {
    const result = await dialog.showOpenDialog(parentWindow, {
      properties: ['openDirectory', 'createDirectory'],
      title: '选择 Ouroboros 工作区',
    });
    if (result.canceled || result.filePaths.length === 0) {
      return '';
    }
    return this.openWorkspace(result.filePaths[0]);
  }

  async createWorkspace(parentWindow) {
    const result = await dialog.showOpenDialog(parentWindow, {
      properties: ['openDirectory', 'createDirectory'],
      title: '选择或创建 Ouroboros 工作区',
    });
    if (result.canceled || result.filePaths.length === 0) {
      return '';
    }
    return this.openWorkspace(result.filePaths[0]);
  }

  async openWorkspace(workspace) {
    this.current = workspace || '';
    await this.rememberWorkspace(this.current);
    return this.current;
  }

  async rememberWorkspace(workspace) {
    if (!workspace || workspace.trim() === '') {
      return;
    }
    this.recent = [workspace, ...this.recent.filter(item => item !== workspace)].slice(0, 10);
    await fs.mkdir(path.dirname(this.statePath), { recursive: true });
    await fs.writeFile(this.statePath, JSON.stringify({
      lastWorkspace: workspace,
      recentWorkspaces: this.recent,
    }, null, 2));
  }

  recentWorkspaces() {
    return [...this.recent];
  }

  snapshot() {
    return { workspace: this.current, recentWorkspaces: this.recentWorkspaces() };
  }
}
