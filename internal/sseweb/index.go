package sseweb

import "net/http"

// indexHTML 是内嵌的测试页面：用于在浏览器中体验 SSE 代理服务器
// （创建会话、发送消息、实时查看事件流、审批命令）。
const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<title>ACTX Agent · SSE</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { background: #0a0a0f; color: #e2e2ea; font: 14px/1.6 "Segoe UI", system-ui, sans-serif;
         min-height: 100vh; padding: 24px; }
  .wrap { max-width: 880px; margin: 0 auto; }
  h1 { font-size: 20px; font-weight: 600; letter-spacing: .5px; margin-bottom: 4px;
       background: linear-gradient(90deg,#8b5cf6,#22d3ee); -webkit-background-clip: text;
       background-clip: text; color: transparent; }
  .sub { color: #6b6b7b; font-size: 12px; margin-bottom: 20px; }
  .bar { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; margin-bottom: 16px; }
  button { background: #17171f; color: #cfcfe0; border: 1px solid #2a2a3a; border-radius: 8px;
           padding: 8px 14px; cursor: pointer; font-size: 13px; transition: all .15s; }
  button:hover { border-color: #8b5cf6; color: #fff; }
  button:disabled { opacity: .4; cursor: not-allowed; }
  button.primary { background: linear-gradient(135deg,#7c3aed,#2563eb); border: none; color: #fff; }
  input[type=text] { flex: 1; min-width: 240px; background: #12121a; color: #e2e2ea;
       border: 1px solid #2a2a3a; border-radius: 8px; padding: 9px 12px; font-size: 13px; outline: none; }
  input[type=text]:focus { border-color: #8b5cf6; }
  #log { background: #0d0d14; border: 1px solid #1e1e2c; border-radius: 12px; padding: 14px;
         height: 58vh; overflow-y: auto; font: 12px/1.7 "Cascadia Code", Consolas, monospace; }
  .ev { margin-bottom: 6px; word-break: break-all; white-space: pre-wrap; }
  .ev .tag { color: #22d3ee; }
  .ev.msg .tag { color: #34d399; }
  .ev.delta .tag { color: #818cf8; }
  .ev.err .tag, .ev.err { color: #f87171; }
  .ev.sys .tag { color: #fbbf24; }
  .ev .txt { color: #9a9ab0; }
  #status { font-size: 12px; margin-left: auto; color: #6b6b7b; }
  #status.on { color: #34d399; }
  ::-webkit-scrollbar { width: 8px; } ::-webkit-scrollbar-thumb { background: #26263a; border-radius: 4px; }
</style>
</head>
<body>
<div class="wrap">
  <h1>ACTX Agent · SSE Server</h1>
  <div class="sub">HTTP + Server-Sent Events 驱动的本地编码代理</div>
  <div class="bar">
    <button id="newBtn" class="primary">新建会话</button>
    <input type="text" id="msg" placeholder="输入消息，回车发送…" autocomplete="off">
    <button id="sendBtn">发送</button>
    <button id="interruptBtn" title="中止当前轮次">中断</button>
    <button id="approveBtn" title="批准待审批命令" style="border-color:#166534">批准</button>
    <button id="denyBtn" title="拒绝待审批命令" style="border-color:#7f1d1d">拒绝</button>
    <span id="status">未连接</span>
  </div>
  <div id="log"></div>
</div>
<script>
const $ = id => document.getElementById(id);
const logEl = $('log');
let sessionId = null, es = null, lastSeq = 0, pendingCallId = null;

function log(kind, tag, text) {
  const div = document.createElement('div');
  div.className = 'ev ' + kind;
  const t = document.createElement('span'); t.className = 'tag'; t.textContent = '[' + tag + '] ';
  div.appendChild(t);
  if (text) { const s = document.createElement('span'); s.className = 'txt'; s.textContent = text; div.appendChild(s); }
  logEl.appendChild(div);
  logEl.scrollTop = logEl.scrollHeight;
}

async function api(method, path, body) {
  const res = await fetch(path, {
    method, headers: body ? {'Content-Type': 'application/json'} : {},
    body: body ? JSON.stringify(body) : undefined
  });
  if (!res.ok) { const e = await res.json().catch(() => ({})); throw new Error(e.error || res.status); }
  return res.status === 204 ? null : res.json();
}

async function newSession() {
  if (es) { es.close(); es = null; }
  try {
    const info = await api('POST', '/api/sessions', {});
    sessionId = info.session_id;
    log('sys', 'session', '会话已创建 · 模型 ' + info.model + ' · ID ' + sessionId.slice(0, 8));
    connect();
  } catch (e) { log('err', 'error', String(e)); }
}

function connect() {
  es = new EventSource('/api/sessions/' + sessionId + '/events');
  es.onopen = () => setStatus('SSE 已连接', true);
  es.onerror = () => setStatus('SSE 重连中…', false);
  es.onmessage = ev => handle(JSON.parse(ev.data));
  // 显式事件类型（event: <type>）也会走到 onmessage 之外的 addEventListener，
  // 因此统一在服务端使用默认 message 之外，这里为所有类型注册兜底。
  es.addEventListener('session_closed', () => setStatus('会话已关闭', false));
}

function setStatus(text, on) {
  const el = $('status'); el.textContent = text; el.className = on ? 'on' : '';
}

let deltaBuf = '';
function handle(entry) {
  lastSeq = entry.seq;
  const m = entry.event.msg || {};
  switch (m.type) {
    case 'agent_message_delta': deltaBuf += m.delta || ''; log('delta', 'delta', m.delta || ''); break;
    case 'agent_message':
      log('msg', 'agent', m.message || deltaBuf); deltaBuf = ''; break;
    case 'reasoning_delta': log('delta', 'think', m.delta || ''); break;
    case 'agent_reasoning': if (m.message) log('sys', 'reasoning', m.message); break;
    case 'user_message': log('msg', 'user', m.message); break;
    case 'exec_command_begin': log('sys', 'exec', '$ ' + (m.command || []).join(' ')); break;
    case 'exec_command_end':
      log('sys', 'output', (m.output || '') + (m.exit_code ? '\n[exit ' + m.exit_code + ']' : '')); break;
    case 'exec_approval_request':
      pendingCallId = m.call_id;
      log('err', 'approval', '需要审批: ' + (m.command || []).join(' ') + ' — 点击 批准/拒绝');
      break;
    case 'token_count': log('sys', 'tokens', 'in ' + m.input_tokens + ' / out ' + m.output_tokens); break;
    case 'turn_started': setStatus('代理运行中…', true); break;
    case 'turn_complete': setStatus('SSE 已连接', true); break;
    case 'stream_error': log('err', 'error', m.error || ''); break;
    case 'session_configured': log('sys', 'config', m.model + ' @ ' + m.provider); break;
    default: log('sys', m.type, m.message || m.error || '');
  }
}

async function send() {
  const text = $('msg').value.trim();
  if (!text || !sessionId) return;
  $('msg').value = '';
  try { await api('POST', '/api/sessions/' + sessionId + '/message', { message: text }); }
  catch (e) { log('err', 'error', String(e)); }
}

async function respondApproval(approved) {
  if (!pendingCallId || !sessionId) return;
  const callId = pendingCallId; pendingCallId = null;
  try { await api('POST', '/api/sessions/' + sessionId + '/approval',
      { call_id: callId, approved, feedback: approved ? '' : 'denied via web' }); }
  catch (e) { log('err', 'error', String(e)); }
}

$('newBtn').onclick = newSession;
$('sendBtn').onclick = send;
$('msg').addEventListener('keydown', e => { if (e.key === 'Enter') send(); });
$('interruptBtn').onclick = async () => {
  if (!sessionId) return;
  try { await api('POST', '/api/sessions/' + sessionId + '/interrupt', {}); } catch (e) { log('err', 'error', String(e)); }
};
$('approveBtn').onclick = () => respondApproval(true);
$('denyBtn').onclick = () => respondApproval(false);
window.addEventListener('beforeunload', () => { if (es) es.close(); });
</script>
</body>
</html>
`

// handleIndex 处理 GET /：返回内嵌测试页面。
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// 仅根路径返回页面，其余未匹配路径返回 404。
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(indexHTML))
}
