import assert from 'node:assert/strict';
import dns from 'node:dns';
import http from 'node:http';
import test from 'node:test';
import { isLoopbackName, LOOPBACK } from './localhost-dns.mjs';

test('a .localhost name reaches loopback and keeps its Host header', async () => {
  const server = http.createServer((request, response) => {
    response.writeHead(200, { 'content-type': 'text/plain' });
    response.end(`${request.headers.host}\n`);
  });
  await new Promise((resolve) => server.listen(0, LOOPBACK, resolve));
  const { port } = server.address();
  try {
    const answer = await fetch(`http://notes.htmlclay.localhost:${port}/`);
    assert.equal(answer.status, 200);
    assert.equal((await answer.text()).trim(), `notes.htmlclay.localhost:${port}`);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
});

test('the patched lookup honours all, and the promise shape', async () => {
  const one = await dns.promises.lookup('notes.htmlclay.localhost');
  assert.deepEqual(one, { address: LOOPBACK, family: 4 });
  const all = await dns.promises.lookup('NOTES.HTMLCLAY.LOCALHOST', { all: true });
  assert.deepEqual(all, [{ address: LOOPBACK, family: 4 }]);
  const callback = await new Promise((resolve, reject) => {
    dns.lookup('localhost', { all: true }, (error, addresses) => (error ? reject(error) : resolve(addresses)));
  });
  assert.deepEqual(callback, [{ address: LOOPBACK, family: 4 }]);
  assert.equal(isLoopbackName('localhost'), true);
  assert.equal(isLoopbackName('example.invalid'), false);
});

test('every other name still goes to the resolver', async () => {
  await assert.rejects(() => dns.promises.lookup('example.invalid'), (error) => error.code !== undefined);
  await assert.rejects(() => dns.promises.lookup('example.invalid', { all: true }));
  const callbackError = await new Promise((resolve) => {
    dns.lookup('example.invalid', (error) => resolve(error));
  });
  assert.notEqual(callbackError, null);
});
