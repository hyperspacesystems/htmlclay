'use strict';

// main.cjs --self-check: loads every part of the service that boots lazily and
// reads files by path, the parts the boot and room tests never reach, and
// prints SELF-CHECK OK. The packed service (build.mjs) ships only the files
// these reads need, so a file the packing misses fails here instead of on a
// person's first framed document, Tailwind page or search.

const fs = require('fs');
const os = require('os');
const path = require('path');
const { Worker } = require('worker_threads');
const { fileURLToPath, pathToFileURL } = require('url');

const app = path.dirname(require.main.filename);

function workerOutcome(worker, until) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => { worker.terminate(); reject(new Error(`no answer in ${until.ms} ms`)); }, until.ms);
    worker.on('message', (m) => {
      if (until.message(m)) { clearTimeout(timer); worker.terminate(); resolve(m); }
    });
    worker.on('error', (e) => { clearTimeout(timer); reject(e); });
    worker.on('exit', (code) => { clearTimeout(timer); reject(new Error(`the worker exited (${code}) before answering`)); });
  });
}

async function selfCheck() {
  const { JSDOM } = require('jsdom');
  const dom = new JSDOM('<!doctype html><p id="x">hi</p>');
  if (dom.window.document.getElementById('x').textContent !== 'hi') throw new Error('jsdom did not parse');

  const { compileTailwind } = require('tailwind-hyperclay');
  const css = await compileTailwind('<html><head><link rel="stylesheet" href="/tailwindcss/x.css"></head><body><div class="prose p-4"><input type="text" class="form-input"></div></body></html>');
  if (!css.includes('.prose') || !css.includes('.p-4')) throw new Error('Tailwind compiled without its typography plugin or utilities');

  const checkpoint = await import('@panphora/clayjs/server/checkpoint');
  if (typeof checkpoint.renderCheckpoint !== 'function') throw new Error('the checkpoint module did not load');
  const rooms = require('@panphora/clayjs/server/rooms');
  if (typeof rooms.splitPage !== 'function') throw new Error('the rooms module did not load');
  await import('@panphora/clayjs/page/yjs');
  await import('hyper-html-api/page');

  const workspace = require('@panphora/hyperclay-workspace/server');
  if (typeof workspace.injectWorkspaceBridge !== 'function') throw new Error('the workspace server module did not load');

  // One empty job, answered: a Worker reports online before it loads its file, so
  // only an answer proves the file is there and loads.
  const compactor = new Worker(pathToFileURL(fileURLToPath(rooms.pageCompactionWorkerUrl())));
  compactor.postMessage({ id: 1, kind: 'tail', sources: [] });
  await workerOutcome(compactor, { message: (m) => m && m.id === 1 && typeof m.update === 'string', ms: 10000 });

  const cachePath = path.join(os.tmpdir(), `htmlclay-self-check-${process.pid}.json`);
  const search = new Worker(path.join(app, 'local/src/main/workspace/search-worker.js'), {
    workerData: { rootPath: path.join(app, 'local/src/main/document-templates'), cachePath },
  });
  try {
    await workerOutcome(search, { message: (m) => m && m.type === 'ready', ms: 20000 });
  } finally {
    fs.rmSync(cachePath, { force: true });
  }
}

module.exports = function runSelfCheck() {
  selfCheck().then(() => {
    console.log('SELF-CHECK OK');
    process.exit(0);
  }, (error) => {
    console.log(`SELF-CHECK FAILED ${error && error.stack ? error.stack : error}`);
    process.exit(1);
  });
};
