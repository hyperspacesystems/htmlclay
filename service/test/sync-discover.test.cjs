'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const { installGate } = require('../sync-net.cjs');
const { createSync, mountSync } = require('../sync.cjs');

const KEY = `hcsk_${'cd34'.repeat(16)}`;
const FEATURES = { accountScopes: true, accountEvents: true, conditionalContent: true, conditionalStructure: true, completeInventory: true };
const ACCOUNTS = [{ accountId: 12, kind: 'personal', username: 'maya' }, { accountId: 40, kind: 'team', username: 'studio' }];

// A server that answers only discovery, as the mode says. The full fake arrives with L1.
function fakeServer(t) {
  const state = { mode: 'ok', seen: [] };
  const server = http.createServer((req, res) => {
    state.seen.push({ path: req.url, key: req.headers['x-api-key'], protocol: req.headers['x-sync-protocol'] });
    const json = (status, body) => { res.writeHead(status, { 'content-type': 'application/json' }); res.end(JSON.stringify(body)); };
    if (req.url !== '/_/sync/accounts') return json(404, { msg: 'not found' });
    switch (state.mode) {
      case 'ok': return json(200, { protocol: 2, features: FEATURES, actor: { id: 12, username: 'maya', person: { id: 'a1', name: 'Maya Lin' } }, accounts: ACCOUNTS });
      case 'no-name': return json(200, { protocol: 2, features: FEATURES, actor: { id: 12, username: 'maya', person: null }, accounts: ACCOUNTS });
      case '401': return json(401, { msg: 'bad key' });
      case '403': return json(403, { msg: 'revoked' });
      case 'old-server': return json(404, { msg: 'no route' });
      case 'partial': return json(200, { protocol: 2, features: { ...FEATURES, completeInventory: false }, actor: { id: 12, username: 'maya' }, accounts: [] });
      case 'garbled': return json(200, { protocol: 2, features: FEATURES, actor: { username: 'maya' } });
      case '500': return json(500, { msg: 'down' });
      default: throw new Error(state.mode);
    }
  });
  t.after(() => server.closeAllConnections() || server.close());
  return new Promise((resolve) => server.listen(0, '127.0.0.1', () => resolve({ url: `http://127.0.0.1:${server.address().port}`, state })));
}

async function connected(t) {
  const server = await fakeServer(t);
  const gate = installGate();
  t.after(() => gate.restore());
  const sync = createSync({ gate });
  assert.equal(await sync.handle({ v: 1, serverUrl: server.url, key: KEY }), 'SYNC KEY HELD');
  return { server, sync };
}

test('discovery answers the actor and the account rows, asking with protocol 2 and the key', async (t) => {
  const { server, sync } = await connected(t);
  assert.deepEqual(await sync.discover(), { ok: true, actor: { id: 12, username: 'maya', fullName: 'Maya Lin' }, accounts: ACCOUNTS });
  assert.deepEqual(server.state.seen, [{ path: '/_/sync/accounts', key: KEY, protocol: '2' }]);
  server.state.mode = 'no-name';
  assert.equal((await sync.discover()).actor.fullName, 'maya');
});

test('a refused key says so', async (t) => {
  const { server, sync } = await connected(t);
  server.state.mode = '401';
  assert.deepEqual(await sync.discover(), { ok: false, code: 'key-refused', status: 401 });
  server.state.mode = '403';
  assert.deepEqual(await sync.discover(), { ok: false, code: 'key-refused', status: 403 });
});

test('an old or partly updated server needs an update', async (t) => {
  const { server, sync } = await connected(t);
  for (const mode of ['old-server', 'partial']) {
    server.state.mode = mode;
    assert.deepEqual(await sync.discover(), { ok: false, code: 'server-update-required' }, mode);
  }
});

test('a garbled answer and a server error', async (t) => {
  const { server, sync } = await connected(t);
  server.state.mode = 'garbled';
  assert.deepEqual(await sync.discover(), { ok: false, code: 'bad-answer' });
  server.state.mode = '500';
  assert.deepEqual(await sync.discover(), { ok: false, code: 'offline', status: 500 });
});

test('no server at all is offline', async (t) => {
  const gate = installGate();
  t.after(() => gate.restore());
  const nowhere = createSync({ gate });
  await nowhere.handle({ v: 1, serverUrl: 'http://127.0.0.1:9', key: KEY });
  assert.deepEqual(await nowhere.discover(), { ok: false, code: 'offline' });
});

test('with no key there is nothing to discover and nothing is sent', async (t) => {
  const server = await fakeServer(t);
  const gate = installGate();
  t.after(() => gate.restore());
  assert.deepEqual(await createSync({ gate }).discover(), { ok: false, code: 'not-connected' });
  assert.deepEqual(server.state.seen, []);
});

test('the mount answers POST /sync/v1/discover and nothing else', async (t) => {
  const { sync } = await connected(t);
  let handler;
  mountSync({ mount: (prefix, h) => { assert.equal(prefix, '/sync/v1/'); handler = h; } }, sync);
  const listener = http.createServer((req, res) => handler(req, res));
  t.after(() => listener.close());
  await new Promise((resolve) => listener.listen(0, '127.0.0.1', resolve));
  const base = `http://127.0.0.1:${listener.address().port}`;
  const call = (method, path, body) => new Promise((resolve, reject) => {
    const req = http.request(`${base}${path}`, { method, headers: { 'content-type': 'application/json' } }, (res) => {
      let text = '';
      res.on('data', (b) => { text += b; });
      res.on('end', () => resolve({ status: res.statusCode, body: JSON.parse(text) }));
    });
    req.on('error', reject);
    req.end(body);
  });
  const ok = await call('POST', '/sync/v1/discover', '{}');
  assert.equal(ok.status, 200);
  assert.equal(ok.body.actor.username, 'maya');
  assert.equal((await call('GET', '/sync/v1/discover')).status, 404);
  assert.equal((await call('POST', '/sync/v1/other', '{}')).status, 404);
  assert.deepEqual(await call('POST', '/sync/v1/discover', '{not json'), { status: 400, body: { ok: false, error: 'bad-request' } });
});
