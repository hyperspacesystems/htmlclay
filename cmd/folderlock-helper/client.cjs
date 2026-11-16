'use strict';

// The Node side of cmd/folderlock-helper. It starts one helper and keeps it for
// the life of the process. The helper's stdin ties every lock to this process:
// when it exits or is killed, stdin closes, the helper exits, and the system
// drops the locks. Hyperclay Local's copy of this file is pulled from here.

const { spawn } = require('node:child_process');
const readline = require('node:readline');

function createFolderLockClient(helperBin, lockDir) {
  const child = spawn(helperBin, [], { stdio: ['pipe', 'pipe', 'inherit'] });
  const pending = new Map();
  let seq = 0;

  const rl = readline.createInterface({ input: child.stdout, crlfDelay: Infinity });
  rl.on('line', (line) => {
    if (!line.trim()) return;
    const msg = JSON.parse(line);
    const waiter = pending.get(msg.reqId);
    if (waiter) {
      pending.delete(msg.reqId);
      waiter.resolve(msg);
    }
  });

  // A helper that cannot start, or that dies, fails every call from then on. The
  // caller decides what that means: Hyperclay Local serves without the lock.
  let gone = null;
  const fail = (err) => {
    gone = err;
    for (const waiter of pending.values()) waiter.reject(err);
    pending.clear();
  };
  child.on('error', fail);
  child.on('exit', (code, signal) => fail(new Error(`folderlock-helper exited (${code ?? signal})`)));
  child.stdin.on('error', () => {});

  function send(payload) {
    if (gone) return Promise.reject(gone);
    const reqId = `r-${++seq}`;
    return new Promise((resolve, reject) => {
      pending.set(reqId, { resolve, reject });
      child.stdin.write(`${JSON.stringify({ ...payload, reqId, lockDir })}\n`);
    });
  }

  return {
    acquire: (id, folder) => send({ op: 'acquire', id, folder }),
    release: (id) => send({ op: 'release', id }),
    close: () => new Promise((resolve) => {
      if (child.exitCode !== null) return resolve();
      child.once('exit', () => resolve());
      child.stdin.end();
    }),
  };
}

module.exports = { createFolderLockClient };
