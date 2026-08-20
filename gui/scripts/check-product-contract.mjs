import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../..', import.meta.url));
const files = [
  'PRODUCT.md',
  'DESIGN.md',
  'docs/superpowers/specs/2026-07-03-gui-rebuild-design.md',
  'docs/superpowers/plans/2026-07-03-desktop-ide-software-master-plan.zh-CN.md',
  '.planning/2026-07-03-gui-rebuild/task_plan.md',
];

const required = [
  '桌面',
  '真实',
  '中文',
];

const forbidden = [
  'safety gate',
  'safety-gate',
  'approval gate',
  'authorization blocker',
  'scope/authorization',
  '安全门控',
  '审批门控',
  '授权阻断',
  '范围校验',
  '人工门控',
];

let failed = false;

for (const file of files) {
  const text = readFileSync(join(root, file), 'utf8');
  for (const phrase of forbidden) {
    if (text.includes(phrase)) {
      console.error(`${file} contains old product direction: ${phrase}`);
      failed = true;
    }
  }
}

const masterPlan = readFileSync(join(root, 'docs/superpowers/plans/2026-07-03-desktop-ide-software-master-plan.zh-CN.md'), 'utf8');
for (const phrase of required) {
  if (!masterPlan.includes(phrase)) {
    console.error(`master plan missing required direction: ${phrase}`);
    failed = true;
  }
}

if (failed) {
  process.exit(1);
}
