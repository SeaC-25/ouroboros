const http = require('http');
function getJSON(url) { return new Promise((res, rej) => { http.get(url, r => { let d=''; r.on('data', c=>d+=c); r.on('end', ()=>res(JSON.parse(d))); }).on('error', rej); }); }
(async () => {
  const pages = await getJSON('http://127.0.0.1:9222/json');
  const page = pages.find(p => p.type === 'page');
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  let id = 0; const pending = {}; const scripts = [];
  ws.onmessage = e => { const m = JSON.parse(e.data); if (m.id && pending[m.id]) { pending[m.id](m); delete pending[m.id]; } if (m.method === 'Debugger.scriptParsed') { scripts.push(m.params); } };
  const send = (method, params={}) => new Promise(res => { const msgId = ++id; pending[msgId] = res; ws.send(JSON.stringify({ id: msgId, method, params })); });
  ws.onopen = async () => {
    await send('Debugger.enable');
    await send('Runtime.enable');
    await send('Page.reload');
    setTimeout(async () => {
      // 找含 index- 的脚本
      const target = scripts.find(s => s.url && s.url.includes('index-'));
      console.log('目标脚本:', target?.url, 'id:', target?.scriptId);
      if (target) {
        const src = await send('Debugger.getScriptSource', { scriptId: target.scriptId });
        const source = src.result?.scriptSource || '';
        const idx = source.indexOf('aiSwarmRuns');
        console.log('aiSwarmRuns 位置:', idx, '总长度:', source.length);
        if (idx >= 0) {
          console.log('=== 前 500 字符 ===');
          console.log(source.slice(Math.max(0,idx-500), idx));
          console.log('=== 后 200 字符 ===');
          console.log(source.slice(idx, idx+200));
        }
        console.log('=== setAISwarmRuns 出现次数 ===');
        console.log((source.match(/setAISwarmRuns/g)||[]).length);
      }
      ws.close(); process.exit(0);
    }, 4000);
  };
})();
