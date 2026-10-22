// Dynamic desktop runtime PID handoff contract driver. Byte-for-byte port of
// checkRuntimePIDContract() from gui/scripts/check-desktop-contract.mjs.
// Usage: node runtime_pid.mjs <repoRoot> <src/main/backend.js> <scripts/desktop_runtime_pid.mjs>
import assert from 'node:assert/strict';
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

const helper = await import(pathToFileURL(process.argv[4]).href);
const lifecycle = await import(pathToFileURL(process.argv[3]).href);
const temporaryRoot = mkdtempSync(join(os.tmpdir(), 'ouro-desktop-pid-contract-'));
const statePath = join(temporaryRoot, 'desktop-runtime-state.json');
const workspace = join(temporaryRoot, 'workspace');
const now = Date.now();
let failed = false;
try {
  writeFileSync(statePath, JSON.stringify({
    status: 'running',
    sessionId: `${now}-${process.pid}`,
    processId: process.pid,
    processStartToken: 'expected-token',
    executablePath: '/expected/Ouroboros',
    heartbeatAt: new Date(now).toISOString(),
    workspace,
  }));
  assert.equal(helper.desktopRuntimeProcessIdFromState(statePath, workspace, {
    now,
    isProcessAlive: () => true,
    readProcessStartToken: () => 'different-token',
  }), 0);
  assert.equal(helper.desktopRuntimeProcessIdFromState(statePath, workspace, {
    now,
    isProcessAlive: () => true,
    readProcessStartToken: () => 'expected-token',
    readProcessExecutablePath: () => '/expected/Ouroboros',
  }), process.pid);
  assert.equal(helper.desktopRuntimeProcessIdFromState(statePath, workspace, {
    now,
    isProcessAlive: () => true,
    readProcessStartToken: () => 'expected-token',
    readProcessExecutablePath: () => '/different/Ouroboros',
  }), 0);

  const manager = new lifecycle.BackendManager({
    app: {
      getPath: () => temporaryRoot,
      on: () => {},
    },
  });
  manager.startDesktopRuntimeSession();
  try {
    assert.equal(helper.desktopRuntimeProcessIdFromState(manager.desktopRuntimeStatePath, '', {
      now: Date.now(),
    }), process.pid);
    assert.equal(existsSync(manager.desktopRuntimeStateTempPath), false);
  } finally {
    manager.stopDesktopRuntimeSession('contract');
  }

  manager.stopDesktopRuntimeSession('fatal-contract', 'crashed');
  manager.stopDesktopRuntimeSession('will-quit');
  const crashedState = JSON.parse(readFileSync(manager.desktopRuntimeStatePath, 'utf8'));
  assert.equal(crashedState.status, 'crashed', 'clean quit hooks must not overwrite a fatal runtime state');
  assert.equal(crashedState.reason, 'will-quit');

  const reusedPIDState = JSON.parse(readFileSync(manager.desktopRuntimeStatePath, 'utf8'));
  writeFileSync(manager.desktopRuntimeStatePath, JSON.stringify({
    ...reusedPIDState,
    status: 'running',
    sessionId: `reused-${process.pid}`,
    processStartToken: 'linux:reused-pid-token',
    heartbeatAt: new Date().toISOString(),
  }));
  const reusedPIDManager = new lifecycle.BackendManager({
    app: {
      getPath: () => temporaryRoot,
      on: () => {},
    },
  });
  reusedPIDManager.startDesktopRuntimeSession();
  try {
    assert.equal(reusedPIDManager.lastDesktopRuntimeIssue?.event, 'desktop.previous_unclean_exit');
  } finally {
    reusedPIDManager.stopDesktopRuntimeSession('contract-reused-pid');
  }
} catch (error) {
  console.error(`desktop runtime PID handoff must reject a reused PID: ${error.message}`);
  failed = true;
} finally {
  rmSync(temporaryRoot, { recursive: true, force: true });
}
if (failed) {
  process.exit(1);
}
