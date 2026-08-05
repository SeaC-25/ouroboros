import { spawn } from 'node:child_process';
import { EventEmitter } from 'node:events';
import net from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

export class BackendManager extends EventEmitter {
  constructor({ app }) {
    super();
    this.app = app;
    this.process = null;
    this.apiBaseURL = '';
    this.workspace = '';
    this.status = 'offline';
    this.lastError = '';
    this.stoppingProcess = null;
    this.failedStartupProcess = null;
  }

  async start(workspace) {
    if (!workspace || workspace.trim() === '') {
      this.status = 'offline';
      this.apiBaseURL = '';
      this.emitStatus();
      return this.snapshot();
    }
    if (this.process && this.workspace === workspace && this.apiBaseURL) {
      return this.snapshot();
    }

    await this.stop();
    const port = await choosePort();
    const command = backendCommand(this.app);
    const args = backendArgs(this.app, workspace, port);

    this.workspace = workspace;
    this.apiBaseURL = `http://127.0.0.1:${port}`;
    this.status = 'starting';
    this.lastError = '';
    this.emitStatus();
    this.process = spawn(command, args, {
      cwd: repoRoot(),
      stdio: ['ignore', 'pipe', 'pipe'],
      env: process.env,
    });

    const child = this.process;
    child.stderr.on('data', chunk => {
      const message = chunk.toString().trim();
      if (message) {
        this.lastError = message.slice(-2000);
      }
    });
    child.once('error', error => {
      if (this.process !== child) {
        return;
      }
      this.process = null;
      this.status = 'crashed';
      this.lastError = error.message;
      this.emitStatus();
    });
    child.once('exit', (code, signal) => {
      if (this.failedStartupProcess === child) {
        return;
      }
      const expectedStop = this.stoppingProcess === child;
      if (this.process === child) {
        this.process = null;
      }
      if (this.stoppingProcess === child) {
        this.stoppingProcess = null;
      }
      if (expectedStop) {
        this.status = 'offline';
        this.emitStatus();
        return;
      }
      this.status = 'crashed';
      this.lastError = this.lastError || describeExit(code, signal);
      this.emitStatus();
    });

    try {
      await waitForHealth(this.apiBaseURL);
    } catch (error) {
      this.failedStartupProcess = child;
      this.process = null;
      await terminateProcess(child);
      this.failedStartupProcess = null;
      this.status = 'crashed';
      this.lastError = error.message;
      this.emitStatus();
      throw error;
    }
    this.status = 'ready';
    this.emitStatus();
    return this.snapshot();
  }

  async initializeWorkspace(workspace) {
    if (!workspace || workspace.trim() === '') {
      throw new Error('workspace path is required');
    }
    const command = backendCommand(this.app);
    const args = backendInitArgs(this.app, workspace);
    await runBackendCommand(command, args);
    return { workspace };
  }

  async stop() {
    if (!this.process) {
      this.status = 'offline';
      this.emitStatus();
      return;
    }
    const child = this.process;
    this.process = null;
    this.stoppingProcess = child;
    await terminateProcess(child);
    if (this.stoppingProcess === child) {
      this.stoppingProcess = null;
    }
    this.status = 'offline';
    this.emitStatus();
  }

  async restart() {
    const workspace = this.workspace;
    if (!workspace || workspace.trim() === '') {
      this.status = 'offline';
      this.emitStatus();
      return this.snapshot();
    }
    await this.stop();
    return this.start(workspace);
  }

  emitStatus() {
    const snapshot = this.snapshot();
    this.emit('status', snapshot);
    return snapshot;
  }

  snapshot() {
    return {
      status: this.status,
      apiBaseURL: this.apiBaseURL,
      workspace: this.workspace,
      running: Boolean(this.process),
      lastError: this.lastError,
    };
  }
}

function describeExit(code, signal) {
  if (signal) {
    return `后端进程收到 ${signal} 后退出`;
  }
  return `后端进程退出，退出码 ${code ?? 'unknown'}`;
}

function terminateProcess(child) {
  return new Promise(resolve => {
    if (!child || child.killed || child.exitCode !== null) {
      resolve();
      return;
    }
    const timeout = setTimeout(() => {
      child.kill('SIGKILL');
      resolve();
    }, 5000);
    child.once('exit', () => {
      clearTimeout(timeout);
      resolve();
    });
    child.kill('SIGTERM');
  });
}

function backendCommand(app) {
  if (!app.isPackaged) {
    return 'go';
  }
  return process.platform === 'win32'
    ? path.join(process.resourcesPath, 'ouro.exe')
    : path.join(process.resourcesPath, 'ouro');
}

function backendArgs(app, workspace, port) {
  if (!app.isPackaged) {
    return ['run', './cmd/ouro', 'serve', workspace, '--port', String(port)];
  }
  return ['serve', workspace, '--port', String(port)];
}

function backendInitArgs(app, workspace) {
  if (!app.isPackaged) {
    return ['run', './cmd/ouro', 'init', workspace];
  }
  return ['init', workspace];
}

function runBackendCommand(command, args) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: repoRoot(),
      stdio: ['ignore', 'pipe', 'pipe'],
      env: process.env,
    });
    let stderr = '';
    child.stderr.on('data', chunk => {
      stderr += chunk.toString();
    });
    child.once('error', reject);
    child.once('exit', code => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(new Error(stderr.trim() || `backend command exited with ${code}`));
    });
  });
}

function repoRoot() {
  return path.resolve(__dirname, '../../..');
}

function choosePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      server.close(() => {
        if (address && typeof address === 'object') {
          resolve(address.port);
          return;
        }
        reject(new Error('unable to choose backend port'));
      });
    });
  });
}

async function waitForHealth(apiBaseURL) {
  const deadline = Date.now() + 30000;
  let lastError;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${apiBaseURL}/api/health`);
      if (response.ok) {
        return;
      }
      lastError = new Error(`backend health returned ${response.status}`);
    } catch (error) {
      lastError = error;
    }
    await new Promise(resolve => setTimeout(resolve, 300));
  }
  throw lastError ?? new Error('backend health timed out');
}
