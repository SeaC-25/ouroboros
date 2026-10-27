// target_server.mjs — minimal login page for agent E2E verification
import { createServer } from 'node:http';

createServer((req, res) => {
	if (req.url === '/redirect') {
		res.writeHead(302, { Location: '/' });
		res.end();
		return;
	}
	res.writeHead(200, { 'Content-Type': 'text/html' });
	res.end(`<!doctype html><html><body>
	<form action="/login" method="post">
	<input type="text" name="userName" /><input type="password" name="loginPass" />
	</form>
	<a href="/admin">admin</a><a href="/docs">docs</a>
	</body></html>`);
}).listen(65299, '127.0.0.1', () => console.log('target ready on 65299'));
