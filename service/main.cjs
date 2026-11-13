'use strict';

// HTML Clay's live-room service: Hyperclay Local's room code (vendor/local, pinned by
// vendor/local.lock.json) as a child of the Go host. Go starts it, so the protocol is
// the trial's (vendor/local/trial/protocol/v1.md, "Transport and bootstrap"): stdin's
// first line is {"v":1,"secret":"<64 hex>"}, answered with READY {"url","epoch","pid"};
// a later {"host":{"url","secret","epoch","roots"}} line is answered with ATTACHED; end
// of stdin shuts the service down. A secret is never an argument or an env var.
//
// Usage: main.cjs --data-dir <absolute dir>
// The data dir is the only place the service may keep files of its own; today it
// keeps none, because every document write goes through Go.
//
// Exit codes: 0 after end of stdin, 2 for bad arguments, 3 for a bad bootstrap line
// or a failure while running.

const fs = require('fs');
const path = require('path');
const readline = require('readline');

function parseArgs(argv) {
  let dataDir = null;
  for (let i = 0; i < argv.length; i += 1) {
    if (argv[i] === '--data-dir' && i + 1 < argv.length) {
      dataDir = argv[i + 1];
      i += 1;
    } else {
      return { error: `unknown argument ${JSON.stringify(argv[i])}` };
    }
  }
  if (!dataDir) return { error: '--data-dir <absolute dir> is required' };
  if (!path.isAbsolute(dataDir)) return { error: '--data-dir must be an absolute path' };
  return { dataDir };
}

class BootstrapError extends Error {}

async function run(dataDir) {
  fs.mkdirSync(dataDir, { recursive: true, mode: 0o700 });
  // Page compaction needs its worker file beside the service; it stays off, as in
  // the trial, until M4 tests it.
  process.env.HYPERCLAY_PAGE_COMPACTION = '0';
  const { createTrialService } = require('./vendor/local/trial/service.cjs');

  const lines = readline.createInterface({ input: process.stdin, crlfDelay: Infinity })[Symbol.asyncIterator]();
  const first = await lines.next();
  if (first.done) throw new BootstrapError('no bootstrap line on stdin');
  let boot;
  try {
    boot = JSON.parse(first.value);
  } catch {
    throw new BootstrapError('the bootstrap line is not JSON');
  }
  if (!boot || boot.v !== 1 || typeof boot.secret !== 'string' || !/^[0-9a-f]{64}$/.test(boot.secret)) {
    throw new BootstrapError('the bootstrap line needs v 1 and a 64-hex secret');
  }
  const service = createTrialService({ secret: boot.secret });
  const url = await service.listen();
  process.stdout.write(`READY ${JSON.stringify({ url, epoch: service.epoch, pid: process.pid })}\n`);
  for (;;) {
    const next = await lines.next();
    if (next.done) break;
    if (!next.value.trim()) continue;
    const message = JSON.parse(next.value);
    if (message.host) {
      await service.attachGo(message.host);
      process.stdout.write('ATTACHED\n');
    }
  }
  await service.close();
}

const args = parseArgs(process.argv.slice(2));
if (args.error) {
  console.error(`[htmlclay-service] ${args.error}`);
  process.exit(2);
}
run(args.dataDir).then(() => process.exit(0), (error) => {
  console.error('[htmlclay-service]', error instanceof BootstrapError ? error.message : (error && error.stack) || error);
  process.exit(3);
});
