'use strict';

// The dashboard's workspace API on the service (shell.cjs): Go forwards an
// admitted /api/ request to /v1/shell/api/... with the bearer, both epochs and the
// dashboard's origin; Local's own routes answer it from the roots Go sent.

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { GO_EPOCH, startService, call } = require('./harness.cjs');

const DASHBOARD = 'http://htmlclay.localhost:1212';
const NOTES = 'http://notes.htmlclay.localhost:1212';
const MANIFEST = require('../../internal/dashboard/assets/local-app/manifest.json');

const shell = (s, pathname, { method = 'GET', origin = DASHBOARD, epoch = true, body } = {}) => call(s, method, `/v1/shell${pathname}`, {
  body,
  headers: {
    ...(origin && { 'x-trial-public-origin': origin }),
    ...(epoch && { 'x-trial-epoch': GO_EPOCH, 'x-trial-peer-epoch': s.ready.epoch }),
  },
});

async function until(read, ok, what) {
  const deadline = Date.now() + 10000;
  for (;;) {
    const value = await read();
    if (ok(value)) return value;
    if (Date.now() > deadline) assert.fail(`${what}: last answer ${JSON.stringify(value)}`);
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
}

async function servedNotes(t) {
  const s = await startService(t);
  const dir = s.folder('notes');
  fs.mkdirSync(path.join(dir, 'sub'));
  fs.writeFileSync(path.join(dir, 'sub', 'b.html'), '<!DOCTYPE html><html><body>b</body></html>');
  fs.mkdirSync(path.join(dir, '.trash'));
  fs.writeFileSync(path.join(dir, '.trash', 'old.html'), 'old');
  const added = await call(s, 'POST', '/v1/roots', { body: { add: [{ id: 'notes', path: dir, url: NOTES }] } });
  assert.deepEqual(added.body.added, ['notes']);
  return { s, dir };
}

test('bootstrap lists the roots Go sent, and the tree and list answer from the folder', async (t) => {
  const { s, dir } = await servedNotes(t);
  const boot = await shell(s, '/api/bootstrap');
  assert.equal(boot.status, 200);
  assert.equal(boot.body.protocol, MANIFEST.protocol);
  assert.equal(boot.body.shellOrigin, DASHBOARD);
  assert.deepEqual(boot.body.account, { signedIn: false, username: null });
  assert.equal(boot.body.roots.length, 1);
  assert.deepEqual({ ...boot.body.roots[0], name: undefined }, { id: 'notes', name: undefined, kind: 'local', origin: NOTES, sync: 'off', teamName: null });

  const tree = await shell(s, '/api/roots/notes/tree');
  assert.equal(tree.status, 200);
  assert.deepEqual(tree.body.entries.map((entry) => entry.path), ['index.html', 'sub', 'sub/b.html']);
  const list = await shell(s, '/api/roots/notes/list');
  assert.deepEqual(list.body.items.map((entry) => entry.path), ['index.html', 'sub']);

  fs.writeFileSync(path.join(dir, 'new.html'), '<!DOCTYPE html><html><body>new</body></html>');
  await until(() => shell(s, '/api/roots/notes/tree'), (answer) => answer.body.entries.some((entry) => entry.path === 'new.html'),
    'a file made on disk shows in the tree');

  const document = await shell(s, '/api/roots/notes/document?path=sub%2Fb.html');
  assert.equal(document.status, 200);
  assert.match(document.body.frameUrl, /^http:\/\/notes\.htmlclay\.localhost:1212\/sub\/b\.html\?workspace-frame=\d+$/);
});

test('commands not switched on answer 501, and a request must come through Go for the dashboard', async (t) => {
  const { s, dir } = await servedNotes(t);
  const before = fs.readdirSync(dir).sort();
  const unknown = await shell(s, '/api/roots/notes/commands', { method: 'POST', body: { command: 'auto-name', path: 'index.html' } });
  assert.equal(unknown.status, 501, 'a command not switched on');
  assert.equal((await shell(s, '/api/roots/notes/upload', { method: 'POST', body: {} })).status, 501, 'uploads are not switched on');
  assert.deepEqual(fs.readdirSync(dir).sort(), before, 'nothing was written');

  assert.equal((await shell(s, '/api/roots', { origin: null })).status, 400);
  assert.equal((await shell(s, '/api/roots', { epoch: false })).status, 409);
  assert.equal((await shell(s, '/api/roots')).status, 200);
  assert.equal((await shell(s, '/api/roots', { origin: 'http://evil.localhost:1212' })).status, 400);
  assert.equal((await call(s, 'GET', '/v1/shell/api/roots', { headers: { authorization: 'Bearer nope' } })).status, 401);
});

test('a root Go stops serving leaves the workspace', async (t) => {
  const { s } = await servedNotes(t);
  assert.equal((await shell(s, '/api/roots')).body.length, 1);
  await call(s, 'POST', '/v1/roots', { body: { remove: ['notes'] } });
  assert.deepEqual((await shell(s, '/api/roots')).body, []);
  assert.equal((await shell(s, '/api/roots/notes/tree')).status, 404);
});
