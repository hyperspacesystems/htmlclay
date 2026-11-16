'use strict';

// 8.16: the vendored sync engine loads in the service's plain Node. Electron is
// not there, so any require('electron') on the way in fails this load.
const Module = require('node:module');
const originalLoad = Module._load;
Module._load = function load(request, ...rest) {
  if (request === 'electron') throw new Error('electron is not available in the service');
  return originalLoad.call(this, request, ...rest);
};

const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');

const VENDOR = path.join(__dirname, '..', 'local');

test('the vendored sync manager and engine load with no Electron', () => {
  const { SyncManager } = require(path.join(VENDOR, 'src', 'main', 'sync-manager.js'));
  const { SyncEngine } = require(path.join(VENDOR, 'src', 'sync-engine', 'index.js'));
  const { followHost, passiveObserver } = require(path.join(VENDOR, 'trial', 'sync-observer.cjs'));
  assert.equal(typeof SyncManager, 'function');
  assert.equal(typeof SyncManager.prototype.bind, 'function');
  assert.equal(typeof SyncEngine, 'function');
  assert.equal(typeof followHost, 'function');
  assert.equal(typeof passiveObserver, 'function');
});
