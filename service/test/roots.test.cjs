'use strict';

// Folders start and stop being served while the service runs (POST /v1/roots):
// an added root answers forwarded requests, a removed one ends its event streams
// and answers 404, and Go learns which roots stayed because a room was unsaved.

const test = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawn } = require('node:child_process');
const { randomBytes } = require('node:crypto');

const MAIN = path.join(__dirname, '..', 'main.cjs');
const GO_EPOCH = 'a1b2c3d4e5f60718';

function stubGo(t) {
  const server = http.createServer((req, res) => {
    if (req.url.startsWith('/v1/events')) {
      res.writeHead(200, { 'content-type': 'application/x-ndjson' });
      res.flushHeaders();
      return;
    }
    res.writeHead(200, { 'content-type': 'application/json' });
    res.end(req.url.startsWith('/v1/unconfirmed') ? '{"documents":[]}' : '{}');
  });
  t.after(() => server.closeAllConnections() || server.close());
  return new Promise((resolve) => server.listen(0, '127.0.0.1', () => resolve(`http://127.0.0.1:${server.address().port}`)));
}

async function startService(t, { attach = true } = {}) {
  const tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-roots-')));
  t.after(() => fs.rmSync(tmp, { recursive: true, force: true }));
  const child = spawn(process.execPath, [MAIN, '--data-dir', path.join(tmp, 'data')], { stdio: ['pipe', 'pipe', 'inherit'] });
  t.after(() => child.exitCode === null && child.kill('SIGKILL'));
  let out = '';
  child.stdout.on('data', (b) => { out += b; });
  const line = (prefix) => new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`no ${prefix} line; stdout ${JSON.stringify(out)}`)), 20000);
    const look = () => {
      const found = out.split('\n').find((l) => l.startsWith(prefix));
      if (found !== undefined) {
        clearTimeout(timer);
        resolve(found);
      }
    };
    child.stdout.on('data', look);
    look();
  });
  const secret = randomBytes(32).toString('hex');
  child.stdin.write(`${JSON.stringify({ v: 1, secret })}\n`);
  const ready = JSON.parse((await line('READY ')).slice('READY '.length));
  if (attach) {
    child.stdin.write(`${JSON.stringify({ host: { url: await stubGo(t), secret: randomBytes(32).toString('hex'), epoch: GO_EPOCH, roots: [] } })}\n`);
    await line('ATTACHED');
  }
  const folder = (name) => {
    const dir = path.join(tmp, name);
    fs.mkdirSync(dir, { recursive: true });
    fs.writeFileSync(path.join(dir, 'index.html'), '<!DOCTYPE html><html><body><p>x</p></body></html>');
    return dir;
  };
  return { ready, secret, folder };
}

function call(s, method, pathname, { body, headers = {}, stream = false } = {}) {
  const url = new URL(s.ready.url);
  return new Promise((resolve, reject) => {
    const req = http.request({
      host: url.hostname, port: url.port, method, path: pathname,
      headers: { authorization: `Bearer ${s.secret}`, ...(body && { 'content-type': 'application/json' }), ...headers },
    }, (res) => {
      if (stream) {
        resolve(res);
        return;
      }
      let text = '';
      res.on('data', (b) => { text += b; });
      res.on('end', () => resolve({ status: res.statusCode, body: text ? JSON.parse(text) : null }));
    });
    req.on('error', reject);
    req.end(body ? JSON.stringify(body) : undefined);
  });
}

const roots = (s, body) => call(s, 'POST', '/v1/roots', { body });
const forwarded = (s, id, host, pathname, opts = {}) => call(s, 'GET', `/v1/route${pathname}`, {
  ...opts,
  headers: { 'x-trial-root': id, 'x-trial-public-host': host, 'x-trial-epoch': GO_EPOCH, 'x-trial-peer-epoch': s.ready.epoch },
});

test('a root added while running answers, and removing it ends its streams', async (t) => {
  const s = await startService(t);
  const host = 'notes.htmlclay.localhost:1212';
  const add = await roots(s, { add: [{ id: 'notes', path: s.folder('notes'), url: `http://${host}` }] });
  assert.deepEqual(add, { status: 200, body: { added: ['notes'], removed: [], kept: [] } });

  const again = await roots(s, { add: [{ id: 'notes', path: s.folder('notes'), url: `http://${host}` }] });
  assert.equal(again.status, 200, 'adding the same root from the same folder changes nothing');

  const streamPath = `/live-sync/stream?lane=saved&document-url=${encodeURIComponent(`http://${host}/index.html`)}`;
  const stream = await forwarded(s, 'notes', host, streamPath, { stream: true });
  assert.equal(stream.statusCode, 200);
  assert.match(String(stream.headers['content-type']), /^text\/event-stream/);
  stream.resume();
  const ended = new Promise((resolve) => stream.on('end', () => resolve('ended')));

  const remove = await roots(s, { remove: ['notes'] });
  assert.deepEqual(remove, { status: 200, body: { added: [], removed: ['notes'], kept: [] } });
  const outcome = await Promise.race([ended, new Promise((resolve) => setTimeout(() => resolve('still open after 3 s'), 3000))]);
  assert.equal(outcome, 'ended');

  const after = await forwarded(s, 'notes', host, '/index.html');
  assert.equal(after.status, 404);
  assert.equal(after.body.error, 'unknown-root');
});

test('two roots on one port are told apart by name', async (t) => {
  const s = await startService(t);
  const add = await roots(s, { add: [
    { id: 'notes', path: s.folder('notes'), url: 'http://notes.htmlclay.localhost:1212' },
    { id: 'journal', path: s.folder('journal'), url: 'http://journal.htmlclay.localhost:1212' },
  ] });
  assert.deepEqual(add.body.added, ['notes', 'journal']);
  await roots(s, { remove: ['journal'] });
  const notes = await forwarded(s, 'notes', 'notes.htmlclay.localhost:1212',
    `/live-sync/stream?lane=saved&document-url=${encodeURIComponent('http://notes.htmlclay.localhost:1212/index.html')}`, { stream: true });
  assert.equal(notes.statusCode, 200, 'the other root still answers');
  notes.destroy();
  assert.equal((await forwarded(s, 'journal', 'journal.htmlclay.localhost:1212', '/index.html')).status, 404);
});

test('refusals: another folder under a served id, a bad body, before attach', async (t) => {
  const s = await startService(t);
  await roots(s, { add: [{ id: 'notes', path: s.folder('notes'), url: 'http://notes.htmlclay.localhost:1212' }] });
  const conflict = await roots(s, { add: [{ id: 'notes', path: s.folder('other'), url: 'http://notes.htmlclay.localhost:1212' }] });
  assert.equal(conflict.status, 409);
  assert.equal(conflict.body.code, 'root-conflict');
  assert.equal((await roots(s, { add: [{ id: 'x', path: '' }] })).status, 400);
  assert.equal((await roots(s, { remove: [7] })).status, 400);
  assert.deepEqual((await roots(s, { remove: ['never-added'] })).body, { added: [], removed: ['never-added'], kept: [] });

  const early = await startService(t, { attach: false });
  assert.equal((await roots(early, { add: [{ id: 'a', path: early.folder('a'), url: 'http://a.htmlclay.localhost:1212' }] })).status, 409);
});
