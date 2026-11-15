'use strict';

// recover.cjs against a stand-in for the service's seams: the admit handler, the
// roots and their lanes, and the authority's rooms. The real service is on the
// other end of Go's TestM3WritersMeet (cmd/htmlclay/writers_meet_test.go).

const test = require('node:test');
const assert = require('node:assert/strict');
const { keepUnsavedEdits } = require('../recover.cjs');

const DOC = '/folder/index.html';

function standIn({ open = true, admitSave } = {}) {
  const calls = [];
  const listeners = new Set();
  const roots = new Map();
  const service = {
    handlers: new Map(),
    roots,
    authority: {
      rooms: new Map(open ? [[DOC, { epoch: 'e', seq: 1 }]] : []),
      async recoverUnsavedEdits(target, token) {
        calls.push(['recover', target.canonicalPath, token]);
      },
    },
    onRoots(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
  service.handlers.set('/v1/admit', async (body, reply) => {
    await null;
    try {
      reply(await roots.get(body.root).lanes.admitSave({ target: { canonicalPath: DOC }, token: body.consultId }));
    } catch (error) {
      reply({ error: error.message }, 500);
    }
  });
  const addRoot = (id) => {
    roots.set(id, {
      lanes: {
        admitSave: admitSave ?? (async () => {
          await null;
          calls.push(['admitSave']);
          return { content: 'admitted' };
        }),
      },
    });
    for (const listener of listeners) listener();
  };
  const admit = (body) => new Promise((resolve) => {
    service.handlers.get('/v1/admit')(body, (answer, status = 200) => resolve({ answer, status }));
  });
  return { service, calls, addRoot, admit };
}

test('a write marked recover keeps the open room\'s edits once the save is admitted', async () => {
  const { service, calls, addRoot, admit } = standIn();
  keepUnsavedEdits(service);
  addRoot('root-0');
  const { answer, status } = await admit({ root: 'root-0', consultId: 'c-1', recover: true });
  assert.deepEqual(calls, [['admitSave'], ['recover', DOC, 'c-1']]);
  assert.deepEqual({ answer, status }, { answer: { content: 'admitted' }, status: 200 });
});

test('a write that is not marked keeps nothing, and neither does one next to it', async () => {
  const { service, calls, addRoot, admit } = standIn();
  keepUnsavedEdits(service);
  addRoot('root-0');
  await Promise.all([
    admit({ root: 'root-0', consultId: 'page', recover: false }),
    admit({ root: 'root-0', consultId: 'program', recover: true }),
    admit({ root: 'root-0', consultId: 'unmarked' }),
  ]);
  assert.deepEqual(calls.filter(([what]) => what === 'recover'), [['recover', DOC, 'program']]);
});

test('a document with no room is admitted and the authority is not asked', async () => {
  const { service, calls, addRoot, admit } = standIn({ open: false });
  keepUnsavedEdits(service);
  addRoot('root-0');
  const { status } = await admit({ root: 'root-0', consultId: 'c-1', recover: true });
  assert.equal(status, 200);
  assert.deepEqual(calls, [['admitSave']]);
});

test('a refused save keeps nothing and is still refused', async () => {
  const { service, calls, addRoot, admit } = standIn({ admitSave: async () => { throw new Error('refused'); } });
  keepUnsavedEdits(service);
  addRoot('root-0');
  const { answer, status } = await admit({ root: 'root-0', consultId: 'c-1', recover: true });
  assert.deepEqual({ answer, status }, { answer: { error: 'refused' }, status: 500 });
  assert.deepEqual(calls, []);
});

test('every root is covered, each once, however often the roots change', async () => {
  const { service, calls, addRoot, admit } = standIn();
  keepUnsavedEdits(service);
  addRoot('root-0');
  addRoot('root-1');
  addRoot('root-2');
  await admit({ root: 'root-0', consultId: 'first', recover: true });
  await admit({ root: 'root-2', consultId: 'last', recover: true });
  assert.deepEqual(calls.filter(([what]) => what === 'recover'), [['recover', DOC, 'first'], ['recover', DOC, 'last']]);
});
