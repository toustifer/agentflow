// Minimal stdio MCP probe for the agent-hub MCP server.
//
// Speaks NDJSON framing — the same framing @modelcontextprotocol/sdk's
// StdioServerTransport uses — so it needs no dependencies and works with a
// plain `node smoke/hub_mcp_probe.mjs`.
//
// Verifies that the server configured as `mcp-hub` in
// ~/.dsh/profiles/web/cordis.patch.yml actually handshakes and publishes
// `hub_login` (and the rest of the hub_* tool surface).
//
// Usage:
//   node smoke/hub_mcp_probe.mjs [serverPath] [nodeExe]
// Exits 0 on success (initialize OK + hub_login listed); non-zero otherwise.

import { spawn } from 'node:child_process';

const TARGET = process.argv[2] || 'D:\\myprogram\\agent-hub\\mcp-server\\index.js';
const NODE = process.argv[3] || process.execPath;

const child = spawn(NODE, [TARGET], {
  stdio: ['pipe', 'pipe', 'pipe'],
  env: { ...process.env, HUB_API_URL: process.env.HUB_API_URL || 'https://hub.stifer.xyz' },
});

let buf = '';
const received = [];
child.stdout.on('data', (d) => {
  buf += d.toString();
  let i;
  while ((i = buf.indexOf('\n')) >= 0) {
    const line = buf.slice(0, i).trim();
    buf = buf.slice(i + 1);
    if (!line) continue;
    try { received.push(JSON.parse(line)); } catch { received.push({ __unparsed: line }); }
  }
});

let stderr = '';
child.stderr.on('data', (d) => { stderr += d.toString(); });

const send = (obj) => child.stdin.write(JSON.stringify(obj) + '\n');

function waitFor(id, ms = 20000) {
  return new Promise((resolve, reject) => {
    const started = Date.now();
    const tick = setInterval(() => {
      const m = received.find((r) => r.id === id);
      if (m) { clearInterval(tick); resolve(m); }
      else if (Date.now() - started > ms) { clearInterval(tick); reject(new Error('timeout waiting id=' + id)); }
    }, 50);
  });
}

let exitCode = 0;
try {
  const t0 = Date.now();
  send({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2024-11-05', capabilities: {}, clientInfo: { name: 'hub-probe', version: '1.0.0' } } });
  const init = await waitFor(1);
  console.log('--- initialize response ---');
  console.log(JSON.stringify(init, null, 2));
  if (!init.result) throw new Error('initialize returned no result');
  send({ jsonrpc: '2.0', method: 'notifications/initialized', params: {} });

  send({ jsonrpc: '2.0', id: 2, method: 'tools/list', params: {} });
  const list = await waitFor(2);
  const tools = list?.result?.tools || [];
  console.log('--- tools/list: ' + tools.length + ' tools ---');
  console.log(tools.map((t) => t.name).join('\n'));
  console.log('--- checks ---');
  const names = tools.map((t) => t.name);
  console.log('HAS_hub_login=' + names.includes('hub_login'));
  console.log('HAS_hub_list_my_businesses=' + names.includes('hub_list_my_businesses'));
  console.log('elapsed_ms=' + (Date.now() - t0));
  if (!names.includes('hub_login')) { console.log('FAIL: hub_login not advertised'); exitCode = 1; }
} catch (e) {
  console.log('PROBE_ERROR: ' + e.message);
  console.log('received_so_far=' + JSON.stringify(received));
  exitCode = 1;
} finally {
  if (stderr.trim()) console.log('--- child stderr ---\n' + stderr.trim());
  child.kill();
  setTimeout(() => process.exit(exitCode), 300);
}
