/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        bg: '#0a0a0a',
        panel: '#111113',
        'panel-2': '#17171a',
        line: '#27272a',
        'line-strong': '#3f3f46',
        text: '#f4f4f5',
        muted: '#a1a1aa',
        soft: '#71717a',
        accent: '#2dd4bf',
        'accent-2': '#f97316',
        danger: '#fb7185',
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
