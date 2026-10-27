// mock_provider.mjs — OpenAI-compatible mock that scripts a model reply
// containing ACTION:OBSERVE + ACTION:BLACKBOARD, to exercise the full
// chat → action → observation → back-link loop without a real key.
//
// Reply selection: the second request (after the observe result returns to
// the model) gets the conclusion reply; the first gets the observe reply.
import { createServer } from 'node:http';

let calls = 0;
createServer((req, res) => {
	let body = '';
	req.on('data', c => (body += c));
	req.on('end', () => {
		calls++;
		const isFollowUp = body.includes('HTTP 观测完成') || body.includes('obs-');
		const content = isFollowUp
			? `观测分析：目标登录页返回 200，表单字段为 userName/loginPass（camelCase），存在 /admin 与 /docs 路径。结论：凭据测试需使用大小写敏感字段名。证据 obs 引用见上。\nACTION:BLACKBOARD 登录页表单字段为 userName/loginPass（camelCase，凭据测试必须保留大小写），含 /admin、/docs 路由 — 基于 obs-1 证据`
			: `我先观测目标登录页。\nACTION:OBSERVE http://127.0.0.1:65299/`;
		res.writeHead(200, { 'Content-Type': 'application/json' });
		res.end(JSON.stringify({
			id: 'chatcmpl-mock',
			object: 'chat.completion',
			choices: [{ index: 0, message: { role: 'assistant', content }, finish_reason: 'stop' }],
			usage: { prompt_tokens: 10, completion_tokens: 20, total_tokens: 30 },
		}));
	});
}).listen(65298, '127.0.0.1', () => console.log('mock provider ready on 65298'));
