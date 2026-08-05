import { chmodSync, copyFileSync, cpSync, existsSync, mkdirSync, renameSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { spawnSync } from 'node:child_process';

const guiRoot = new URL('..', import.meta.url).pathname;
const root = new URL('../..', import.meta.url).pathname;
const dryRun = process.argv.includes('--dry-run');
const packageDirMode = process.argv.includes('--dir');
const binaryName = process.platform === 'win32' ? 'ouro.exe' : 'ouro';
const resourceBinary = join(guiRoot, 'dist', 'resources', binaryName);
const desktopDir = join(guiRoot, 'dist', 'desktop', `Ouroboros-${process.platform}-${process.arch}`);
const electronDist = join(guiRoot, 'node_modules', 'electron', 'dist');
const packagedAppLayout = 'resources/app';
const packagedAppDir = join(desktopDir, 'resources', 'app');
const packagedBackend = join(desktopDir, 'resources', binaryName);
const packageScriptName = 'package:desktop:dir';

const steps = packageDirMode
  ? [
    `npm --prefix ${guiRoot} run build`,
    `mkdir -p ${dirname(resourceBinary)}`,
    `BIN=${resourceBinary} ${join(root, 'scripts', 'build.sh')}`,
    `rm -rf ${desktopDir}`,
    `copyElectronRuntime ${electronDist} ${desktopDir}`,
    `mkdir -p ${packagedAppDir}`,
    `copy ${join(guiRoot, 'dist', 'renderer')} ${join(packagedAppDir, 'dist', 'renderer')}`,
    `copy ${join(guiRoot, 'dist', 'main')} ${join(packagedAppDir, 'dist', 'main')}`,
    `write ${join(packagedAppDir, 'package.json')}`,
    `copy ${resourceBinary} ${packagedBackend}`,
  ]
  : [
    `mkdir -p ${dirname(resourceBinary)}`,
    `BIN=${resourceBinary} ${join(root, 'scripts', 'build.sh')}`,
  ];

if (dryRun) {
  for (const step of steps) {
    console.log(step);
  }
  process.exit(0);
}

if (packageDirMode) {
  runFrontendBuild();
}

buildBackend(resourceBinary);
assertFile(resourceBinary, 'desktop backend binary');

if (!packageDirMode) {
  console.log(`desktop backend ready: ${resourceBinary}`);
  process.exit(0);
}

copyElectronRuntime(electronDist, desktopDir);
copyAppBundle();
copyFileSync(resourceBinary, packagedBackend);
chmodSync(packagedBackend, 0o755);
assertFile(packagedBackend, 'packaged desktop backend binary');

console.log(`${packageScriptName}: ${desktopDir}`);

function runFrontendBuild() {
  const result = spawnSync('npm', ['run', 'build'], {
    cwd: guiRoot,
    stdio: 'inherit',
  });
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}

function buildBackend(output) {
  mkdirSync(dirname(output), { recursive: true });
  const result = spawnSync(join(root, 'scripts', 'build.sh'), {
    cwd: root,
    stdio: 'inherit',
    env: {
      ...process.env,
      BIN: output,
    },
  });
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}

function copyElectronRuntime(source, destination) {
  if (!existsSync(source)) {
    console.error(`Electron runtime not found: ${source}`);
    process.exit(1);
  }
  rmSync(destination, { recursive: true, force: true });
  mkdirSync(dirname(destination), { recursive: true });
  cpSync(source, destination, { recursive: true });
  renameElectronBinary(destination);
}

function renameElectronBinary(destination) {
  const sourceName = process.platform === 'win32' ? 'electron.exe' : 'electron';
  const targetName = process.platform === 'win32' ? 'Ouroboros.exe' : 'Ouroboros';
  const sourcePath = join(destination, sourceName);
  const targetPath = join(destination, targetName);
  if (existsSync(sourcePath)) {
    renameSync(sourcePath, targetPath);
    if (process.platform !== 'win32') {
      chmodSync(targetPath, 0o755);
    }
  }
}

function copyAppBundle() {
  mkdirSync(packagedAppDir, { recursive: true });
  cpSync(join(guiRoot, 'dist', 'renderer'), join(packagedAppDir, 'dist', 'renderer'), { recursive: true });
  cpSync(join(guiRoot, 'dist', 'main'), join(packagedAppDir, 'dist', 'main'), { recursive: true });
  const rootEnv = join(root, '.env');
  if (existsSync(rootEnv)) {
    copyFileSync(rootEnv, join(packagedAppDir, '.env'));
  }
  writeFileSync(join(packagedAppDir, 'package.json'), JSON.stringify({
    name: 'ouroboros-desktop',
    productName: 'Ouroboros',
    version: '0.1.0',
    type: 'module',
    main: 'dist/main/main.js',
  }, null, 2) + '\n');
}

function assertFile(path, label) {
  if (!existsSync(path)) {
    console.error(`${label} was not created: ${path}`);
    process.exit(1);
  }
  const stats = statSync(path);
  if (!stats.isFile() || stats.size === 0) {
    console.error(`${label} is invalid: ${path}`);
    process.exit(1);
  }
}
