'use strict';

// Starts the service as Go does (READY, then an attach line naming a stub Go)
// and calls its listener with the bearer.

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

async function startService(t, { attach = true, go = null, roots = [], env = {} } = {}) {
  const tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-roots-')));
  t.after(() => fs.rmSync(tmp, { recursive: true, force: true }));
  const child = spawn(process.execPath, [MAIN, '--data-dir', path.join(tmp, 'data')], { stdio: ['pipe', 'pipe', 'inherit'], env: { ...process.env, ...env } });
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
    child.stdin.write(`${JSON.stringify({ host: { url: go || await stubGo(t), secret: randomBytes(32).toString('hex'), epoch: GO_EPOCH, roots } })}\n`);
    await line('ATTACHED');
  }
  const folder = (name) => {
    const dir = path.join(tmp, name);
    fs.mkdirSync(dir, { recursive: true });
    fs.writeFileSync(path.join(dir, 'index.html'), '<!DOCTYPE html><html><body><p>x</p></body></html>');
    return dir;
  };
  const send = (message) => child.stdin.write(`${JSON.stringify(message)}\n`);
  return { ready, secret, folder, tmp, send, line, output: () => out };
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

module.exports = { GO_EPOCH, stubGo, startService, call };
