'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawn } = require('node:child_process');
const { randomBytes } = require('node:crypto');
const { stubGo, GO_EPOCH, removeAfterExit } = require('./harness.cjs');
const { installGate } = require('../sync-net.cjs');

const MAIN = path.join(__dirname, '..', 'main.cjs');
const KEY = `hcsk_${'ab12'.repeat(16)}`;

// A server that records every request it is sent, standing in for hyperclay.com.
function recordingServer(t) {
  const seen = [];
  const server = http.createServer((req, res) => {
    seen.push(`${req.method} ${req.url}`);
    res.writeHead(200, { 'content-type': 'application/json' });
    res.end('{"ok":true}');
  });
  t.after(() => server.closeAllConnections() || server.close());
  return new Promise((resolve) => server.listen(0, '127.0.0.1', () => resolve({ url: `http://127.0.0.1:${server.address().port}`, seen })));
}

// The service after ATTACHED, with every stdout line kept in order.
async function attached(t) {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-sync-key-'));
  const child = spawn(process.execPath, [MAIN, '--data-dir', path.join(tmp, 'data')], { stdio: ['pipe', 'pipe', 'pipe'] });
  removeAfterExit(t, child, tmp);
  let out = '';
  let err = '';
  child.stdout.on('data', (b) => { out += b; });
  child.stderr.on('data', (b) => { err += b; });
  const lines = () => out.split('\n').filter(Boolean);
  const nth = (n) => new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`no line ${n}; stdout ${JSON.stringify(out)} stderr ${JSON.stringify(err)}`)), 20000);
    const look = () => {
      if (lines().length > n) {
        clearTimeout(timer);
        child.stdout.off('data', look);
        resolve(lines()[n]);
      }
    };
    child.stdout.on('data', look);
    look();
  });
  child.stdin.write(`${JSON.stringify({ v: 1, secret: randomBytes(32).toString('hex') })}\n`);
  await nth(0);
  child.stdin.write(`${JSON.stringify({ host: { url: await stubGo(t), secret: randomBytes(32).toString('hex'), epoch: GO_EPOCH, roots: [] } })}\n`);
  assert.equal(await nth(1), 'ATTACHED');
  let next = 2;
  const send = async (line) => {
    child.stdin.write(`${line}\n`);
    const answer = await nth(next);
    next += 1;
    return answer;
  };
  return { child, send, output: () => out + err };
}

test('the key line is answered HELD, a clear CLEARED, a bad line REFUSED, and none is ever echoed', async (t) => {
  const s = await attached(t);
  assert.equal(await s.send(JSON.stringify({ sync: { v: 1, serverUrl: 'https://hyperclay.com', key: KEY } })), 'SYNC KEY HELD');
  assert.equal(await s.send(JSON.stringify({ sync: { v: 1, key: null } })), 'SYNC KEY CLEARED');
  const refused = [
    `{"sync":{"v":1,"serverUrl":"https://hyperclay.com","key":"${KEY}"`,
    JSON.stringify({ sync: { v: 2, serverUrl: 'https://hyperclay.com', key: KEY } }),
    JSON.stringify({ sync: { v: 1, serverUrl: 'http://hyperclay.com', key: KEY } }),
    JSON.stringify({ sync: { v: 1, serverUrl: 'https://user:pw@hyperclay.com', key: KEY } }),
    JSON.stringify({ sync: { v: 1, serverUrl: 'https://hyperclay.com', key: `${KEY}0` } }),
    JSON.stringify({ sync: { v: 1, serverUrl: 'https://hyperclay.com', key: 'hcsk_short' } }),
  ];
  for (const line of refused) {
    assert.equal(await s.send(line), 'SYNC KEY REFUSED', line);
  }
  assert.ok(!s.output().includes('ab12ab12'), 'no part of a key reached stdout or stderr');
  assert.equal(s.child.exitCode, null, 'the service is still running');
});

test('the gate', async (t) => {
  const server = await recordingServer(t);
  const other = await recordingServer(t);
  const gate = installGate();
  t.after(() => gate.restore());

  await t.test('no request before the key', async () => {
    await assert.rejects(fetch(`${server.url}/sync/v1/discover`), { code: 'sync-host-refused' });
    assert.deepEqual(server.seen, []);
    assert.equal(gate.refusedCount(), 1);
  });

  await t.test('the server passes while a key is held', async () => {
    gate.hold(server.url);
    const answer = await fetch(`${server.url}/sync/v1/discover`);
    assert.equal(answer.status, 200);
    assert.deepEqual(server.seen, ['GET /sync/v1/discover']);
  });

  await t.test('another host is refused, loopback included', async () => {
    await assert.rejects(fetch(`${other.url}/x`), { code: 'sync-host-refused' });
    await assert.rejects(fetch(new URL('/x', other.url)), { code: 'sync-host-refused' });
    await assert.rejects(fetch(new Request(`${other.url}/x`)), { code: 'sync-host-refused' });
    assert.deepEqual(other.seen, []);
    assert.equal(gate.refusedCount(), 4);
  });

  await t.test('a redirect to another host is refused, so the key never follows it', async () => {
    const hop = http.createServer((req, res) => {
      res.writeHead(302, { location: `${other.url}/stolen` });
      res.end();
    });
    t.after(() => hop.closeAllConnections() || hop.close());
    const hopURL = await new Promise((resolve) => hop.listen(0, '127.0.0.1', () => resolve(`http://127.0.0.1:${hop.address().port}`)));
    gate.hold(hopURL);
    await assert.rejects(fetch(`${hopURL}/sync/v1/discover`, { headers: { authorization: 'Bearer test' } }));
    assert.deepEqual(other.seen, []);
    gate.hold(server.url);
  });

  await t.test('a cleared key closes the gate again', async () => {
    gate.clear();
    await assert.rejects(fetch(`${server.url}/again`), { code: 'sync-host-refused' });
    assert.deepEqual(server.seen, ['GET /sync/v1/discover']);
  });
});
