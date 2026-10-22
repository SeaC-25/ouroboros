/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        'bg': 'var(--bg)',
        'panel': 'var(--panel)',
        'panel-2': 'var(--panel-2)',
        'panel-3': 'var(--panel-3)',
        'line': 'var(--line)',
        'line-strong': 'var(--line-strong)',
        'text': 'var(--text)',
        'muted': 'var(--muted)',
        'soft': 'var(--soft)',
        'accent': 'var(--brand-red)',
        'accent-muted': 'var(--red-dim)',
        'success': 'var(--green)',
        'warning': 'var(--yellow)',
        'danger': 'var(--red)',
      },
      fontFamily: {
        sans: ['Inter', 'ui-sans-serif', 'system-ui', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace'],
      },
      animation: {
        'spin-slow': 'spin 20s linear infinite',
      },
    },
  },
  plugins: [],
}
