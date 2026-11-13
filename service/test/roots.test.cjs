'use strict';

// Folders start and stop being served while the service runs (POST /v1/roots):
// an added root answers forwarded requests, a removed one ends its event streams
// and answers 404, and Go learns which roots stayed because a room was unsaved.

const test = require('node:test');
const assert = require('node:assert/strict');
const { GO_EPOCH, startService, call } = require('./harness.cjs');

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
