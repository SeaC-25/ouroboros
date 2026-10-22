import { existsSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

export function parseOuroEnv(content) {
  const values = {};
  for (const rawLine of content.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith('#') || !line.includes('=')) {
      continue;
    }
    const separator = line.indexOf('=');
    const key = line.slice(0, separator).trim();
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) {
      continue;
    }
    let value = line.slice(separator + 1).trim();
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1);
    }
    values[key] = value;
  }
  return values;
}

export function loadOuroEnv({ app, env = process.env } = {}) {
  const candidates = [
    path.resolve(__dirname, '..', '..', '..', '.env'),
    path.resolve(process.cwd(), '.env'),
  ];
  if (process.resourcesPath) {
    candidates.push(path.resolve(process.resourcesPath, '.env'));
  }
  if (app && typeof app.getAppPath === 'function') {
    candidates.push(path.resolve(app.getAppPath(), '.env'));
  }
  if (app && typeof app.getPath === 'function') {
    candidates.push(path.resolve(app.getPath('userData'), '.ouroboros.env'));
    candidates.push(path.resolve(app.getPath('userData'), '.env'));
  }

  const loaded = [];
  for (const candidate of candidates) {
    try {
      if (!existsSync(candidate)) {
        continue;
      }
      const parsed = parseOuroEnv(readFileSync(candidate, 'utf8'));
      let changed = false;
      for (const [key, value] of Object.entries(parsed)) {
        if (key in env) {
          continue;
        }
        env[key] = value;
        changed = true;
      }
      if (changed) {
        loaded.push(candidate);
      }
    } catch {
      // 环境文件只是便捷配置，加载失败不应阻断桌面端启动。
    }
  }
  return loaded;
}
