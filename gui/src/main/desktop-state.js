import fs from 'node:fs/promises';
import path from 'node:path';

const DEFAULT_WINDOW_STATE = {
  width: 1280,
  height: 820,
};

export class DesktopStateManager {
  constructor({ app }) {
    this.app = app;
    this.statePath = path.join(app.getPath('userData'), 'window-state.json');
    this.windowState = { ...DEFAULT_WINDOW_STATE };
  }

  async loadWindowState() {
    try {
      const state = JSON.parse(await fs.readFile(this.statePath, 'utf8'));
      this.windowState = normalizeWindowState(state);
    } catch {
      this.windowState = { ...DEFAULT_WINDOW_STATE };
    }
    return { ...this.windowState };
  }

  async saveWindowState(window) {
    if (!window || window.isDestroyed()) {
      return { ...this.windowState };
    }
    const bounds = window.getBounds();
    this.windowState = {
      x: bounds.x,
      y: bounds.y,
      width: bounds.width,
      height: bounds.height,
      maximized: window.isMaximized(),
    };
    await fs.mkdir(path.dirname(this.statePath), { recursive: true });
    await fs.writeFile(this.statePath, JSON.stringify(this.windowState, null, 2));
    return { ...this.windowState };
  }

  snapshot() {
    return { windowState: { ...this.windowState } };
  }
}

function normalizeWindowState(state) {
  const width = positiveInteger(state?.width, DEFAULT_WINDOW_STATE.width);
  const height = positiveInteger(state?.height, DEFAULT_WINDOW_STATE.height);
  const normalized = { width, height };
  if (Number.isInteger(state?.x)) {
    normalized.x = state.x;
  }
  if (Number.isInteger(state?.y)) {
    normalized.y = state.y;
  }
  if (typeof state?.maximized === 'boolean') {
    normalized.maximized = state.maximized;
  }
  return normalized;
}

function positiveInteger(value, fallback) {
  return Number.isInteger(value) && value > 0 ? value : fallback;
}
