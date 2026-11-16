'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawn } = require('node:child_process');
const { randomBytes } = require('node:crypto');
const { removeAfterExit } = require('./harness.cjs');

const MAIN = path.join(__dirname, '..', 'main.cjs');

// A stand-in for Go's control API: it keeps /v1/events open and answers anything
// else with an empty success, recording what the service asked for.
function stubGo(t) {
  const seen = [];
  const server = http.createServer((req, res) => {
    seen.push(`${req.method} ${req.url.split('?')[0]}`);
    if (req.url.startsWith('/v1/events')) {
      res.writeHead(200, { 'content-type': 'application/x-ndjson' });
      res.flushHeaders();
      return;
    }
    res.writeHead(200, { 'content-type': 'application/json' });
    res.end('{}');
  });
  t.after(() => server.closeAllConnections() || server.close());
  return new Promise((resolve) => server.listen(0, '127.0.0.1', () => resolve({ url: `http://127.0.0.1:${server.address().port}`, seen })));
}

function start(t, args) {
  const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-service-'));
  const child = spawn(process.execPath, [MAIN, ...(args ?? ['--data-dir', path.join(dataDir, 'data')])], { stdio: ['pipe', 'pipe', 'pipe'] });
  removeAfterExit(t, child, dataDir);
  let out = '';
  let err = '';
  child.stdout.on('data', (b) => { out += b; });
  child.stderr.on('data', (b) => { err += b; });
  const exitCode = new Promise((resolve) => child.on('exit', (code) => resolve(code)));
  // An exit that never comes fails the test instead of hanging it.
  const exited = Promise.race([exitCode, new Promise((resolve) => setTimeout(() => resolve('still running after 10 s'), 10000).unref())]);
  const line = (prefix, ms = 20000) => new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`no ${prefix} line in ${ms} ms; stdout ${JSON.stringify(out)} stderr ${JSON.stringify(err)}`)), ms);
    const look = () => {
      const found = out.split('\n').find((l) => l.startsWith(prefix));
      if (found !== undefined) {
        clearTimeout(timer);
        child.stdout.off('data', look);
        resolve(found);
      }
    };
    child.stdout.on('data', look);
    look();
  });
  return { child, line, exited, dataDir, output: () => ({ out, err }) };
}

test('bootstrap, READY, ATTACHED, then exit 0 at end of stdin', async (t) => {
  const go = await stubGo(t);
  const s = start(t);
  s.child.stdin.write(`${JSON.stringify({ v: 1, secret: randomBytes(32).toString('hex') })}\n`);
  const ready = JSON.parse((await s.line('READY ')).slice('READY '.length));
  assert.match(ready.url, /^http:\/\/127\.0\.0\.1:\d+$/);
  assert.match(ready.epoch, /^[0-9a-f]+$/);
  assert.equal(ready.pid, s.child.pid);
  assert.ok(fs.statSync(path.join(s.dataDir, 'data')).isDirectory(), 'the data dir was created');

  s.child.stdin.write(`${JSON.stringify({ host: { url: go.url, secret: randomBytes(32).toString('hex'), epoch: 'a1b2c3d4e5f60718', roots: [] } })}\n`);
  assert.equal(await s.line('ATTACHED'), 'ATTACHED');

  s.child.stdin.end();
  assert.equal(await s.exited, 0, JSON.stringify(s.output()));
});

test('a request without the secret is refused', async (t) => {
  const s = start(t);
  s.child.stdin.write(`${JSON.stringify({ v: 1, secret: randomBytes(32).toString('hex') })}\n`);
  const ready = JSON.parse((await s.line('READY ')).slice('READY '.length));
  const res = await fetch(`${ready.url}/v1/flush`, { method: 'POST', body: '{}' });
  assert.ok(res.status === 401 || res.status === 403, `status ${res.status}`);
  s.child.stdin.end();
  assert.equal(await s.exited, 0);
});

for (const [name, line] of [
  ['a short secret', JSON.stringify({ v: 1, secret: 'abc' })],
  ['the wrong version', JSON.stringify({ v: 2, secret: 'a'.repeat(64) })],
  ['a line that is not JSON', 'hello'],
]) {
  test(`exit 3 without READY on ${name}`, async (t) => {
    const s = start(t);
    s.child.stdin.write(`${line}\n`);
    assert.equal(await s.exited, 3);
    assert.doesNotMatch(s.output().out, /READY/);
  });
}

test('exit 3 when stdin ends before a bootstrap line', async (t) => {
  const s = start(t);
  s.child.stdin.end();
  assert.equal(await s.exited, 3);
});

test('exit 2 without --data-dir or with a relative one', async (t) => {
  assert.equal(await start(t, []).exited, 2);
  assert.equal(await start(t, ['--data-dir', 'relative/dir']).exited, 2);
});
