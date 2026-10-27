// normalize-colors.mjs — map hardcoded colors in ouroboros-ide.css onto
// semantic CSS variables so a light theme can override them wholesale.
import { readFileSync, writeFileSync } from 'node:fs';

const CSS = 'src/styles/ouroboros-ide.css';
let css = readFileSync(CSS, 'utf8');

// Buckets: dark-ish surface hexes collapse onto existing panel variables.
// Accent reds keep the brand hue; overlays become rgb(var(--overlay-*)).
const surface = new Map([
  ['#100808', 'var(--bg)'],
  ['#110809', 'var(--bg)'],
  ['#120708', 'var(--bg)'],
  ['#120a0b', 'var(--bg-2)'],
  ['#12090b', 'var(--bg-2)'],
  ['#120909', 'var(--bg-2)'],
  ['#140a0c', 'var(--bg-2)'],
  ['#140b0d', 'var(--bg-2)'],
  ['#150b0d', 'var(--panel)'],
  ['#160b0d', 'var(--panel)'],
  ['#160c0e', 'var(--panel)'],
  ['#171011', 'var(--panel-2)'],
  ['#180d0f', 'var(--panel-2)'],
  ['#180c0f', 'var(--panel-2)'],
  ['#18090a', 'var(--panel-2)'],
  ['#190d10', 'var(--panel-2)'],
  ['#1a0e10', 'var(--panel-2)'],
  ['#1b0e10', 'var(--panel-2)'],
  ['#1c0e10', 'var(--panel-2)'],
  ['#1d0e11', 'var(--panel-2)'],
  ['#201014', 'var(--panel-3)'],
  ['#221014', 'var(--panel-3)'],
  ['#261116', 'var(--panel-3)'],
  ['#281216', 'var(--panel-3)'],
  ['#2a1014', 'var(--panel-3)'],
  ['#2a1216', 'var(--panel-3)'],
  ['#2d1719', 'var(--panel-3)'],
  ['#2f171a', 'var(--panel-3)'],
  ['#341a1e', 'var(--panel-3)'],
  ['#3a1d21', 'var(--panel-3)'],
  ['#5a2b2f', 'var(--line-strong)'],
  ['#3b1014', 'var(--red-dim)'],
  ['#a3303a', 'var(--red)'],
  ['#6e3038', 'var(--red-dim)'],
  ['#ff6971', 'var(--red-bright)'],
  ['#ff737a', 'var(--red-bright)'],
  ['#e8d3d4', 'var(--muted)'],
  ['#e8c7ca', 'var(--muted)'],
  ['#d8aeb2', 'var(--soft)'],
  ['#66717e', 'var(--soft)'],
  ['#556274', 'var(--soft)'],
  ['#56343a', 'var(--line-strong)'],
  ['#b9e6ca', 'var(--green)'],
  ['#fff6f6', 'var(--text)'],
]);

// Overlay rgb values -> rgb triplet variables (alpha preserved inline).
const overlays = new Map([
  ['25, 8, 11', 'var(--overlay-panel-rgb)'],
  ['24, 8, 10', 'var(--overlay-panel-rgb)'],
  ['22, 9, 11', 'var(--overlay-panel-rgb)'],
  ['26, 9, 11', 'var(--overlay-panel-rgb)'],
  ['239, 63, 73', 'var(--brand-red-rgb)'],
  ['220, 80, 60', 'var(--brand-red-rgb)'],
  ['215, 80, 60', 'var(--brand-red-rgb)'],
  ['240, 75, 75', 'var(--brand-red-rgb)'],
  ['70, 199, 99', 'var(--green-rgb)'],
  ['0, 0, 0', 'var(--overlay-black-rgb)'],
  ['8, 3, 4', 'var(--overlay-black-rgb)'],
  ['10, 5, 6', 'var(--overlay-black-rgb)'],
  ['16, 8, 10', 'var(--overlay-black-rgb)'],
  ['255, 255, 255', 'var(--overlay-white-rgb)'],
  ['255,255,255', 'var(--overlay-white-rgb)'],
  ['210, 170, 174', 'var(--overlay-muted-rgb)'],
  ['28, 12, 14', 'var(--overlay-panel-rgb)'],
  ['58, 20, 24', 'var(--line-strong)'],
  ['55, 16, 22', 'var(--red-dim)'],
  ['70, 10, 16', 'var(--red-dim)'],
  ['40, 12, 16', 'var(--overlay-panel-rgb)'],
  ['35, 12, 16', 'var(--overlay-panel-rgb)'],
]);

let hexReplaced = 0;
for (const [hex, replacement] of surface) {
  const pattern = new RegExp(hex.replace('#', '#'), 'gi');
  css = css.replace(pattern, () => {
    hexReplaced += 1;
    return replacement;
  });
}

let overlayReplaced = 0;
css = css.replace(/rgba?\((\d+,\s*\d+,\s*\d+)(?:,\s*([\d.]+))?\)/g, (match, rgb, alpha) => {
  const normalized = rgb.replace(/\s+/g, ' ').replace(/, /g, ', ');
  const mapped = overlays.get(normalized) ?? overlays.get(rgb.replace(/\s/g, ''));
  if (!mapped) return match;
  overlayReplaced += 1;
  return alpha !== undefined ? `rgba(${mapped}, ${alpha})` : `rgb(${mapped})`;
});

writeFileSync(CSS, css);
console.log(`hex replaced: ${hexReplaced}, overlays replaced: ${overlayReplaced}`);
