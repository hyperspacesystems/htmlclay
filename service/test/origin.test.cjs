'use strict';

// The service half of the served-folder gate: Local's own app, as the service
// mounts it for a root Go serves at http://<name>.htmlclay.localhost:<port>,
// refuses every origin and Host but that root's own. Same table as the Go gate's
// (internal/served/gates_name_test.go TestGateByName).

const test = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const { createApp } = require('../vendor/local/src/main/server.js');

const OWN = 'notes.htmlclay.localhost:1212';

async function startApp(t, origin) {
  const dir = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-origin-')));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  fs.writeFileSync(path.join(dir, 'index.html'), '<!DOCTYPE html><html><body><p>x</p></body></html>');
  const root = { id: 'notes', path: dir, canonicalPath: dir, port: 1212, ...(origin && { origin }) };
  const app = createApp({ root, devHooks: null, isKnownPath: null });
  const server = http.createServer(app);
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  t.after(() => server.closeAllConnections() || server.close());
  return server.address().port;
}

// Sends one request with exactly these headers (Host included) and answers its status.
function send(port, method, pathname, headers) {
  return new Promise((resolve, reject) => {
    const req = http.request({ host: '127.0.0.1', port, method, path: pathname, headers, setHost: false }, (res) => {
      res.resume();
      resolve(res.statusCode);
    });
    req.on('error', reject);
    req.end(method === 'POST' ? '<!DOCTYPE html><html><body><p>y</p></body></html>' : undefined);
  });
}

const write = (origin, host = OWN) => ({ Host: host, 'Content-Type': 'text/html', ...(origin && { Origin: origin }) });

test('a root with a public origin accepts only its own', async (t) => {
  const port = await startApp(t, `http://${OWN}`);
  const cases = [
    ['own origin write', write(`http://${OWN}`), true],
    ['own origin in upper case', write('http://NOTES.htmlclay.localhost:1212'), true],
    ["another folder's origin", write('http://journal.htmlclay.localhost:1212'), false],
    ["a loose file's origin", write('http://htmlclay3.localhost:1212'), false],
    ["the dashboard's origin", write('http://htmlclay.localhost:1212'), false],
    ['own name on another port', write('http://notes.htmlclay.localhost:4321'), false],
    ['a bare loopback origin', write('http://127.0.0.1:1212'), false],
    ['Origin null', write('null'), false],
    ['a script: no Origin', write(undefined), true],
    ['wrong Host', write(`http://${OWN}`, 'journal.htmlclay.localhost:1212'), false],
    ['bare loopback Host', write(undefined, '127.0.0.1:1212'), false],
    ['rebound Host', write(`http://${OWN}`, 'evil.example'), false],
    ['upper-case Host', write(undefined, 'Notes.HTMLClay.localhost:1212'), true],
  ];
  for (const [name, headers, ok] of cases) {
    const status = await send(port, 'POST', '/_/save/index.html', headers);
    assert.equal(status !== 403, ok, `${name}: status ${status}`);
  }
});

test('a root with only a port keeps the loopback-and-port rule', async (t) => {
  const port = await startApp(t, null);
  const host = `localhost:${port}`;
  assert.notEqual(await send(port, 'POST', '/_/save/index.html', write(`http://${host}`, host)), 403);
  assert.notEqual(await send(port, 'POST', '/_/save/index.html', write(`http://127.0.0.1:${port}`, host)), 403);
  assert.equal(await send(port, 'POST', '/_/save/index.html', write(`http://localhost:${port + 1}`, host)), 403);
  assert.equal(await send(port, 'POST', '/_/save/index.html', write(undefined, OWN)), 403);
});
