import { cpSync, mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const guiRoot = fileURLToPath(new URL('..', import.meta.url));
const dest = join(guiRoot, 'dist', 'main');
mkdirSync(dest, { recursive: true });
cpSync(join(guiRoot, 'src', 'main'), dest, { recursive: true, force: true });
