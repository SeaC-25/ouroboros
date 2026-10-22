const esbuild = require('esbuild');
(async () => {
  const r = await esbuild.build({
    entryPoints: ['F:/Ouroboros/Ouroboros/gui/repro/test.tsx'],
    bundle: true, write: false, minify: false, jsx: 'automatic', target: 'es2020',
    loader: { '.tsx': 'tsx', '.ts': 'ts' },
    external: ['react', 'react/jsx-runtime'],
  });
  const code = r.outputFiles[0].text;
  console.log('=== 输出 ===');
  console.log(code);
})();
