const http = require('http');
function getJSON(url) { return new Promise((res, rej) => { http.get(url, r => { let d=''; r.on('data', c=>d+=c); r.on('end', ()=>res(JSON.parse(d))); }).on('error', rej); }); }
(async () => {
  const pages = await getJSON('http://127.0.0.1:9222/json');
  const page = pages.find(p => p.type === 'page');
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  let id = 0; const pending = {};
  ws.onmessage = e => { const m = JSON.parse(e.data); if (m.id && pending[m.id]) { pending[m.id](m); delete pending[m.id]; } };
  const send = (method, params={}) => new Promise(res => { const msgId = ++id; pending[msgId] = res; ws.send(JSON.stringify({ id: msgId, method, params })); });
  ws.onopen = async () => {
    await send('Runtime.enable');
    await send('Debugger.enable');
    // 获取所有脚本, 找含 aiSwarmRuns 的
    const { result: { scriptIds } } = await send('Debugger.getScriptSource', {});
    // 列出脚本
    const scripts = await send('Runtime.evaluate', { expression: 'performance.getEntriesByType("resource").map(r=>r.name).join("\\n")', returnByValue: true });
    console.log('RESOURCES:', scripts.result?.result?.value?.slice(0, 500));
    // 尝试在页面上下文检查 typeof aiSwarmRuns (会在组件作用域外, 应 undefined, 但确认)
    const evalRes = await send('Runtime.evaluate', { expression: 'JSON.stringify({ typeofAiSwarmRuns: typeof aiSwarmRuns, hasReactRoot: !!document.getElementById("root").children.length })', returnByValue: true });
    console.log('EVAL:', evalRes.result?.result?.value);
    ws.close(); process.exit(0);
  };
})();
