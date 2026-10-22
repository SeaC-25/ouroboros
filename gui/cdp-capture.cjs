const http = require('http');
function getJSON(url) {
  return new Promise((res, rej) => {
    http.get(url, r => { let d=''; r.on('data', c=>d+=c); r.on('end', ()=>res(JSON.parse(d))); }).on('error', rej);
  });
}
(async () => {
  const pages = await getJSON('http://127.0.0.1:9222/json');
  const page = pages.find(p => p.type === 'page');
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  let id = 0;
  const pending = {};
  ws.onmessage = (event) => {
    const m = JSON.parse(event.data);
    if (m.id && pending[m.id]) { pending[m.id](m); delete pending[m.id]; return; }
    if (m.method === 'Runtime.exceptionThrown') {
      console.log('EXCEPTION:', JSON.stringify(m.params.exceptionDetails?.exception?.description || m.params.exceptionDetails?.text));
    }
    if (m.method === 'Runtime.consoleAPICalled' && m.params.type === 'error') {
      console.log('CONSOLE_ERROR:', JSON.stringify(m.params.args?.map(a=>a.value||a.description||'').join(' ')));
    }
  };
  const send = (method, params={}) => new Promise((res) => {
    const msgId = ++id;
    pending[msgId] = res;
    ws.send(JSON.stringify({ id: msgId, method, params }));
  });
  ws.onopen = async () => {
    await send('Runtime.enable');
    await send('Page.enable');
    await send('Page.reload');
    setTimeout(async () => {
      const evalRes = await send('Runtime.evaluate', { expression: `JSON.stringify({
        hasRoot: !!document.getElementById('root'),
        rootChildren: document.getElementById('root')?.children.length ?? -1,
        rootHTML: (document.getElementById('root')?.innerHTML || '').slice(0, 300),
        bodyChildren: document.body.children.length,
        hasBridge: !!window.ouroboros,
        bridgeMethods: window.ouroboros ? Object.keys(window.ouroboros) : [],
        title: document.title
      })`, returnByValue: true });
      console.log('DOM_STATE:', evalRes.result?.result?.value);
      ws.close(); process.exit(0);
    }, 3500);
  };
})();
