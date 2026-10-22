// Vite dev-server driver for devmgr. Mirrors startViteDevServer +
// viteDevServerURL from gui/scripts/dev-desktop.mjs, then stays alive until
// the parent terminates it. The resolved development URL is announced on
// stdout with the OURO_DEV_SERVER_URL marker.
import { createServer } from 'vite';

const marker = 'OURO_DEV_SERVER_URL';
const guiRoot = process.argv[2];
const logLevel = process.argv[3] || undefined;

const server = await createServer({
  root: guiRoot,
  ...(logLevel ? { logLevel } : {}),
});
await server.listen();

const candidate = server?.resolvedUrls?.local?.[0] || server?.resolvedUrls?.network?.[0] || '';
if (!candidate) {
  console.error('Vite did not report a desktop development URL.');
  process.exit(1);
}
const url = new URL(candidate);
if (!['http:', 'https:'].includes(url.protocol)) {
  console.error(`Unsupported Vite development URL: ${candidate}`);
  process.exit(1);
}
console.log(`${marker} ${url.href}`);

process.on('SIGTERM', () => process.exit(143));
process.on('SIGINT', () => process.exit(130));
await new Promise(() => {});
