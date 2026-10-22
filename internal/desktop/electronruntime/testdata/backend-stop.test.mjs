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

  // Windows 无法向控制台子进程投递 POSIX SIGTERM:child.kill() 会立即终止进程,
  // 因此不存在"正在终止且仍被追踪"的时间窗(上面的 waitFor 已证明终止期间确实
  // 设置过 stoppingProcess)。POSIX 下断言该窗口存在——退出清理必须仍能看到
  // sidecar 的运行状态与 PID。
  if (process.platform !== 'win32') {
    assert.equal(manager.snapshot().running, true, 'terminating sidecar must remain visible to quit cleanup');
    assert.equal(manager.snapshot().processId, child.pid, 'terminating sidecar PID must remain available');
  }

  let secondStopSettled = false;
  secondStop = manager.stop().finally(() => {
    secondStopSettled = true;
  });
  await delay(100);
  // Windows 上终止是同步的,并发 stop 可能已经结算;POSIX 下必须等待在途终止。
  if (process.platform !== 'win32') {
    assert.equal(secondStopSettled, false, 'concurrent stop must await the in-flight termination');
  }

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
