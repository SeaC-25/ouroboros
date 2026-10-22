// Dynamic user desktop installer contract driver. Byte-for-byte port of
// checkUserDesktopInstallerContract() (plus its helpers) from
// gui/scripts/check-desktop-contract.mjs. Requires sh (Linux/macOS).
// Usage: node installer.mjs <repoRoot>
import assert from 'node:assert/strict';
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readlinkSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';

const repoRoot = process.argv[2];
const installer = join(repoRoot, 'scripts', 'install_desktop_app.sh');
let failed = false;

function createDesktopPackageFixture(packageDir) {
  const files = {
    'Ouroboros': '#!/bin/sh\nprintf "%s\\n" "$0"\n',
    'resources/ouro': '#!/bin/sh\nexit 0\n',
    'resources/app/package.json': '{"name":"ouroboros-contract","main":"dist/main/main.js","type":"module"}\n',
    'resources/app/dist/main/main.js': 'import "./memory-profile.js";\n',
    'resources/app/dist/main/application-main.js': 'import "./backend.js";\n',
    'resources/app/dist/main/preload.js': 'require("electron");\n',
    'resources/app/dist/main/backend.js': 'import "./base-backend.js";\n',
    'resources/app/dist/main/base-backend.js': 'export class BackendManager {}\n',
    'resources/app/dist/main/memory-profile.js': 'export const mode = "low";\n',
    'resources/app/dist/main/workspace.js': 'export class WorkspaceManager {}\n',
    'resources/app/dist/main/desktop-state.js': 'export class DesktopStateManager {}\n',
    'resources/app/dist/main/assets/ouroboros-icon.png': 'ouroboros-snake-icon\n',
    'resources/app/dist/renderer/index.html': '<!doctype html><title>Ouroboros</title>\n',
    'resources.pak': 'resources\n',
    'icudtl.dat': 'icu\n',
    'locales/en-US.pak': 'locale\n',
  };
  for (const [relativePath, contents] of Object.entries(files)) {
    const fullPath = join(packageDir, relativePath);
    mkdirSync(dirname(fullPath), { recursive: true });
    writeFileSync(fullPath, contents);
  }
  chmodSync(join(packageDir, 'Ouroboros'), 0o755);
  chmodSync(join(packageDir, 'resources', 'ouro'), 0o755);
}

function runInstaller(packageDir, prefix, dataHome) {
  return spawnSync('sh', [installer, packageDir], {
    encoding: 'utf8',
    env: {
      ...process.env,
      HOME: dirname(prefix),
      PREFIX: prefix,
      XDG_DATA_HOME: dataHome,
    },
  });
}

function desktopExecEscape(value) {
  return String(value).replaceAll('\\', '\\\\').replaceAll('"', '\\"').replaceAll('`', '\\`').replaceAll('$', '\\$').replaceAll('%', '%%');
}

if (!existsSync(installer)) {
  console.error('missing user desktop installer: scripts/install_desktop_app.sh');
  process.exit(1);
}

const temporaryRoot = mkdtempSync(join(os.tmpdir(), 'ouro-desktop-install-contract-'));
const packageDir = join(temporaryRoot, "package with ' quote");
const prefix = join(temporaryRoot, "prefix with ' quote");
const dataHome = join(temporaryRoot, "data with ' quote");
try {
  createDesktopPackageFixture(packageDir);
  rmSync(join(packageDir, 'resources', 'app', 'dist', 'main', 'memory-profile.js'));
  const rejected = runInstaller(packageDir, prefix, dataHome);
  assert.notEqual(rejected.status, 0, 'an incomplete composed Electron main bundle must be rejected');

  writeFileSync(join(packageDir, 'resources', 'app', 'dist', 'main', 'memory-profile.js'), '');
  const emptyRejected = runInstaller(packageDir, prefix, dataHome);
  assert.notEqual(emptyRejected.status, 0, 'an empty composed Electron main module must be rejected');

  writeFileSync(join(packageDir, 'resources', 'app', 'dist', 'main', 'memory-profile.js'), 'export const mode = "low";\n');
  const installed = runInstaller(packageDir, prefix, dataHome);
  assert.equal(installed.status, 0, installed.stderr || installed.stdout);

  const currentLink = join(dataHome, 'ouroboros-desktop', 'current');
  const launcher = join(prefix, 'bin', 'ouroboros-desktop');
  const desktopFile = join(dataHome, 'applications', 'dev.ouroboros.Ouroboros.desktop');
  const desktop = readFileSync(desktopFile, 'utf8');
  assert.ok(readlinkSync(currentLink).startsWith('releases/package-'));
  assert.ok(desktop.includes(`Exec="${desktopExecEscape(launcher)}" %U`));
  assert.ok(desktop.includes(`Icon=${currentLink}/resources/app/dist/main/assets/ouroboros-icon.png`));
  assert.ok(desktop.includes('StartupNotify=false'));
  const launched = spawnSync(launcher, ['--contract'], { encoding: 'utf8' });
  assert.equal(launched.status, 0, launched.stderr);
  assert.equal(resolve(launched.stdout.trim()), resolve(currentLink, 'Ouroboros'));
} catch (error) {
  console.error(`user desktop installer must install and launch the real packaged app: ${error.message}`);
  failed = true;
} finally {
  rmSync(temporaryRoot, { recursive: true, force: true });
}
if (failed) {
  process.exit(1);
}
