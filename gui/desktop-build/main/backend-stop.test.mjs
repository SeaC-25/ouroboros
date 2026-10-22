import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { EventEmitter, once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { BackendManager } from '../src/main/backend.js';

const userData = mkdtempSync(path.join(os.tmpdir(), 'ouro-backend-stop-'));
const app = new EventEmitter();
app.getPath = name => name === 'userData' ? userData : process.execPath;

const manager = new BackendManager({ app });
const child = spawn(process.execPath, ['-e', `
process.on('SIGTERM', () => setTimeout(() => process.exit(0), 500));
process.stdout.write('ready\\n');
setInterval(() => {}, 1000);
`], { stdio: ['ignore', 'pipe', 'ignore'] });

let firstStop;
let secondStop;
try {
  await once(child.stdout, 'data');
  manager.process = child;
  manager.workspace = '/workspace/concurrent-stop';
  manager.apiBaseURL = 'http://127.0.0.1:1';
  manager.status = 'ready';

  firstStop = manager.stop();
  await waitFor(() => manager.stoppingProcess === child);
  assert.equal(manager.snapshot().running, true, 'terminating sidecar must remain visible to quit cleanup');
  assert.equal(manager.snapshot().processId, child.pid, 'terminating sidecar PID must remain available');

  let secondStopSettled = false;
  secondStop = manager.stop().finally(() => {
    secondStopSettled = true;
  });
  await delay(100);
  assert.equal(secondStopSettled, false, 'concurrent stop must await the in-flight termination');

  await Promise.all([firstStop, secondStop]);
  assert.equal(manager.snapshot().running, false);
  assert.equal(manager.snapshot().processId, 0);
} finally {
  await Promise.allSettled([firstStop, secondStop].filter(Boolean));
  if (child.exitCode === null && child.signalCode === null) {
    child.kill('SIGKILL');
    await once(child, 'exit');
  }
  rmSync(userData, { recursive: true, force: true });
}

console.log('backend concurrent stop contract: ok');

function delay(milliseconds) {
  return new Promise(resolve => setTimeout(resolve, milliseconds));
}

async function waitFor(predicate) {
  const deadline = Date.now() + 2000;
  while (Date.now() < deadline) {
    if (predicate()) {
      return;
    }
    await delay(10);
  }
  assert.fail('timed out waiting for sidecar termination');
}
