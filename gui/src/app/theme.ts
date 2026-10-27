// theme.ts — three-state theme switch (light | dark | system).
// The choice persists in localStorage; "system" follows the OS via
// matchMedia and applies data-theme on <html> so the CSS variable layers
// in ouroboros-ide.css take effect without any component changes.

export type ThemeMode = 'light' | 'dark' | 'system';

const STORAGE_KEY = 'ouro-theme';

export function readStoredTheme(): ThemeMode {
  try {
    const value = localStorage.getItem(STORAGE_KEY);
    if (value === 'light' || value === 'dark' || value === 'system') {
      return value;
    }
  } catch {
    // localStorage unavailable (private mode): fall through to system.
  }
  return 'system';
}

export function storeTheme(mode: ThemeMode): void {
  try {
    localStorage.setItem(STORAGE_KEY, mode);
  } catch {
    // Persistence is best-effort; the in-memory switch still applies.
  }
}

function systemPrefersLight(): boolean {
  return typeof window.matchMedia === 'function'
    && window.matchMedia('(prefers-color-scheme: light)').matches;
}

export function resolveTheme(mode: ThemeMode): 'light' | 'dark' {
  if (mode === 'system') {
    return systemPrefersLight() ? 'light' : 'dark';
  }
  return mode;
}

export function applyTheme(mode: ThemeMode): void {
  const effective = resolveTheme(mode);
  const root = document.documentElement;
  if (effective === 'light') {
    root.setAttribute('data-theme', 'light');
  } else {
    root.removeAttribute('data-theme');
  }
  root.style.colorScheme = effective;
}

/** Activate theme handling: apply the stored choice and follow OS changes
 *  while the app is open. Returns a disposer for tests/unmount. */
export function activateTheme(): () => void {
  applyTheme(readStoredTheme());
  if (typeof window.matchMedia !== 'function') {
    return () => undefined;
  }
  const query = window.matchMedia('(prefers-color-scheme: light)');
  const listener = () => {
    if (readStoredTheme() === 'system') {
      applyTheme('system');
    }
  };
  query.addEventListener('change', listener);
  return () => query.removeEventListener('change', listener);
}

export function setTheme(mode: ThemeMode): void {
  storeTheme(mode);
  applyTheme(mode);
}
