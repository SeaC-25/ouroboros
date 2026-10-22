const esbuild = require('esbuild');
(async () => {
  const r = await esbuild.build({
    entryPoints: ['F:/Ouroboros/Ouroboros/gui/src/app/OuroborosIDE.tsx'],
    bundle: true, write: false, minify: false, jsx: 'automatic', target: 'es2020',
    loader: { '.tsx': 'tsx', '.ts': 'ts' },
    external: ['react', 'react/jsx-runtime'],
  });
  const code = r.outputFiles[0].text;
  console.log('=== swarmData 出现 ===');
  const re = /swarmData2?/g; let m;
  while ((m = re.exec(code))) console.log(`[${m[0]}] @ ${m.index}: ${code.slice(Math.max(0,m.index-45), m.index+45).replace(/\n/g,' ')}`);
})();
